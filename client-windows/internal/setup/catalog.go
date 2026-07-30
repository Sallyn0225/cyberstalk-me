// Package setup backs `agent.exe -setup`, the local configuration UI.
//
// PRIVACY: this package holds the only structure in the agent that keeps raw
// window titles around — the catalog of what has been in the foreground, which
// is what makes the UI able to show "you were just in this window, what should
// it be called?". The rules that constrain it:
//
//   - The catalog lives in memory and is discarded when the process exits.
//     Persisting it would break the agent's core promise that raw titles never
//     touch disk.
//   - Titles are never logged, not even at debug level.
//   - Titles leave the process only through the loopback HTTP API, to the
//     person sitting at the machine, and never through the report contract.
//
// The package is pure Go with no Win32 and no build constraint: the foreground
// source is injected by the caller. That keeps the whole thing testable
// anywhere, and keeps the one Windows-only dependency in cmd/agent.
package setup

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Defaults for Catalog. The sample cap is what keeps a long -setup session from
// growing without bound: a browser alone can produce a new title per page.
const (
	DefaultMaxSamplesPerApp = 20
	DefaultMaxApps          = 256
)

// TitleSample is one distinct window title observed for an application.
type TitleSample struct {
	Title     string    `json:"title"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Count     int       `json:"count"`
}

// App is one application that has been in the foreground.
type App struct {
	// Process is the executable base name, lowercased ("code.exe").
	Process   string    `json:"process"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Count     int       `json:"count"`
	// Samples are the distinct titles seen for this application, most recent
	// first.
	Samples []TitleSample `json:"samples"`
	// Configurable reports whether a rule may be written for this entry. It is
	// false for the placeholder an elevated window produces: that name is not a
	// real executable, so a rule keyed on it could never match anything.
	Configurable bool `json:"configurable"`
}

// CatalogSnapshot is an immutable view of the catalog, safe to hand to a
// serializer while observation continues.
type CatalogSnapshot struct {
	// Apps are ordered most recently seen first, which is the order the UI
	// wants: switch to an app and it appears at the top.
	Apps []App `json:"apps"`
	// Locked reports whether the most recent observation was the lock screen.
	// The UI explains why nothing is being discovered while it is true.
	Locked bool `json:"locked"`
	// LockedSeen counts lock-screen observations, so the UI can say the lock
	// screen was seen at all rather than only reporting the current state.
	LockedSeen int `json:"locked_seen"`
	// Observations counts every sample fed in, including locked ones.
	Observations int `json:"observations"`
}

// Catalog accumulates the applications and window titles seen in the
// foreground. It is safe for concurrent use: one goroutine observes while HTTP
// handlers read.
type Catalog struct {
	mu   sync.Mutex
	apps map[string]*App

	maxSamples   int
	maxApps      int
	now          func() time.Time
	locked       bool
	lockedSeen   int
	observations int
}

// CatalogOptions configures a Catalog. The zero value is usable: every field
// falls back to its default.
type CatalogOptions struct {
	MaxSamplesPerApp int
	MaxApps          int
	// Now is injected so tests can control ageing without sleeping.
	Now func() time.Time
}

// NewCatalog returns an empty catalog.
func NewCatalog(opts CatalogOptions) *Catalog {
	c := &Catalog{
		apps:       make(map[string]*App),
		maxSamples: opts.MaxSamplesPerApp,
		maxApps:    opts.MaxApps,
		now:        opts.Now,
	}
	if c.maxSamples <= 0 {
		c.maxSamples = DefaultMaxSamplesPerApp
	}
	if c.maxApps <= 0 {
		c.maxApps = DefaultMaxApps
	}
	if c.now == nil {
		c.now = time.Now
	}
	return c
}

// Observe records one foreground reading.
//
// An empty process means there was no foreground window, which is the lock
// screen or a session switch; nothing is added to the catalog and no title is
// read. An empty title is recorded as an application sighting without a sample:
// plenty of windows have no title at all.
func (c *Catalog) Observe(process, title string) {
	key := strings.ToLower(strings.TrimSpace(process))
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.observations++

	if key == "" {
		c.locked = true
		c.lockedSeen++
		return
	}
	c.locked = false

	app := c.apps[key]
	if app == nil {
		app = &App{Process: key, FirstSeen: now, Configurable: isConfigurable(key)}
		c.apps[key] = app
	}
	app.LastSeen = now
	app.Count++
	if title != "" {
		app.addSample(title, now, c.maxSamples)
	}
	c.evictApps()
}

// addSample merges title into the app's samples, keeping them most-recent
// first and bounded.
func (a *App) addSample(title string, now time.Time, max int) {
	for i := range a.Samples {
		if a.Samples[i].Title == title {
			a.Samples[i].LastSeen = now
			a.Samples[i].Count++
			s := a.Samples[i]
			a.Samples = append(a.Samples[:i], a.Samples[i+1:]...)
			a.Samples = append([]TitleSample{s}, a.Samples...)
			return
		}
	}
	a.Samples = append([]TitleSample{{
		Title: title, FirstSeen: now, LastSeen: now, Count: 1,
	}}, a.Samples...)
	if len(a.Samples) > max {
		// Samples are ordered most recent first, so the oldest is last.
		a.Samples = a.Samples[:max]
	}
}

// evictApps drops the least recently seen applications once the map is over
// its cap. Called with the lock held.
func (c *Catalog) evictApps() {
	if len(c.apps) <= c.maxApps {
		return
	}
	keys := make([]string, 0, len(c.apps))
	for k := range c.apps {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return c.apps[keys[i]].LastSeen.Before(c.apps[keys[j]].LastSeen)
	})
	for _, k := range keys[:len(c.apps)-c.maxApps] {
		delete(c.apps, k)
	}
}

// Snapshot returns a deep copy of the catalog. The copy matters: handlers
// serialize it while the observer keeps writing.
func (c *Catalog) Snapshot() CatalogSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := CatalogSnapshot{
		Apps:         make([]App, 0, len(c.apps)),
		Locked:       c.locked,
		LockedSeen:   c.lockedSeen,
		Observations: c.observations,
	}
	for _, app := range c.apps {
		clone := *app
		clone.Samples = append([]TitleSample(nil), app.Samples...)
		snap.Apps = append(snap.Apps, clone)
	}
	sort.Slice(snap.Apps, func(i, j int) bool {
		if snap.Apps[i].LastSeen.Equal(snap.Apps[j].LastSeen) {
			// Deterministic order for apps observed within the same tick,
			// otherwise the UI list would jitter between polls.
			return snap.Apps[i].Process < snap.Apps[j].Process
		}
		return snap.Apps[i].LastSeen.After(snap.Apps[j].LastSeen)
	})
	return snap
}

// Titles returns the distinct titles recorded for one process, most recent
// first. It is what the regexp tester matches against.
func (c *Catalog) Titles(process string) []string {
	key := strings.ToLower(strings.TrimSpace(process))
	c.mu.Lock()
	defer c.mu.Unlock()

	app := c.apps[key]
	if app == nil {
		return nil
	}
	titles := make([]string, len(app.Samples))
	for i, s := range app.Samples {
		titles[i] = s.Title
	}
	return titles
}

// isConfigurable reports whether a rule can be written for this process name.
//
// collect reports a foreground window whose process it could not identify (an
// elevated window denies OpenProcess) as a placeholder containing characters
// that are illegal in a Windows file name. Testing for those characters rather
// than for the placeholder's exact spelling keeps this package free of a
// dependency on the Win32-only collect package, and is the same property that
// guarantees the placeholder can never collide with a real rule.
func isConfigurable(process string) bool {
	return !strings.ContainsAny(process, `?*<>|"/\`)
}
