package setup

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// clock is a controllable time source; observation ordering is the whole point
// of the catalog, so tests must not depend on wall-clock resolution.
type clock struct{ t time.Time }

func newClock() *clock {
	return &clock{t: time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) now() time.Time  { return c.t }
func (c *clock) tick() time.Time { c.t = c.t.Add(time.Second); return c.t }

func newTestCatalog(t *testing.T, opts CatalogOptions) (*Catalog, *clock) {
	t.Helper()
	c := newClock()
	opts.Now = c.now
	return NewCatalog(opts), c
}

func TestObserveAggregatesByProcess(t *testing.T) {
	cat, clk := newTestCatalog(t, CatalogOptions{})
	cat.Observe("Code.exe", "main.go - project - VS Code")
	clk.tick()
	cat.Observe("code.exe", "README.md - project - VS Code")
	clk.tick()
	cat.Observe("code.exe", "main.go - project - VS Code") // repeat of the first

	snap := cat.Snapshot()
	if len(snap.Apps) != 1 {
		t.Fatalf("len(Apps) = %d, want 1 (case-insensitive aggregation)", len(snap.Apps))
	}
	app := snap.Apps[0]
	if app.Process != "code.exe" {
		t.Errorf("Process = %q, want the lowercased base name", app.Process)
	}
	if app.Count != 3 {
		t.Errorf("Count = %d, want 3", app.Count)
	}
	if len(app.Samples) != 2 {
		t.Fatalf("len(Samples) = %d, want 2 distinct titles", len(app.Samples))
	}
	// The repeated title is both the most recent and seen twice.
	if app.Samples[0].Title != "main.go - project - VS Code" || app.Samples[0].Count != 2 {
		t.Errorf("Samples[0] = %+v, want the repeated title with Count 2", app.Samples[0])
	}
	if !app.Samples[0].FirstSeen.Before(app.Samples[0].LastSeen) {
		t.Errorf("repeat did not advance LastSeen: %+v", app.Samples[0])
	}
	if !app.Configurable {
		t.Error("a normal executable should be configurable")
	}
}

// R2.3: switching to an app puts it at the top of the list.
func TestSnapshotOrdersMostRecentFirst(t *testing.T) {
	cat, clk := newTestCatalog(t, CatalogOptions{})
	for _, p := range []string{"a.exe", "b.exe", "c.exe"} {
		cat.Observe(p, "t")
		clk.tick()
	}
	cat.Observe("a.exe", "t again")

	snap := cat.Snapshot()
	got := make([]string, len(snap.Apps))
	for i, a := range snap.Apps {
		got[i] = a.Process
	}
	if want := "a.exe,c.exe,b.exe"; strings.Join(got, ",") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}

// R2.5: a browser can invent a new title per page; memory must stay bounded.
func TestSampleCapEvictsOldest(t *testing.T) {
	cat, clk := newTestCatalog(t, CatalogOptions{MaxSamplesPerApp: 3})
	for i := 0; i < 10; i++ {
		cat.Observe("chrome.exe", fmt.Sprintf("page %d", i))
		clk.tick()
	}
	samples := cat.Snapshot().Apps[0].Samples
	if len(samples) != 3 {
		t.Fatalf("len(Samples) = %d, want the cap of 3", len(samples))
	}
	for i, want := range []string{"page 9", "page 8", "page 7"} {
		if samples[i].Title != want {
			t.Errorf("Samples[%d] = %q, want %q", i, samples[i].Title, want)
		}
	}
}

// A title that keeps coming back must not be evicted just because it was first
// seen long ago — recency is what matters.
func TestSampleCapKeepsRefreshedTitles(t *testing.T) {
	cat, clk := newTestCatalog(t, CatalogOptions{MaxSamplesPerApp: 2})
	cat.Observe("chrome.exe", "inbox")
	clk.tick()
	cat.Observe("chrome.exe", "docs")
	clk.tick()
	cat.Observe("chrome.exe", "inbox") // refreshed, now the most recent
	clk.tick()
	cat.Observe("chrome.exe", "news") // pushes out "docs", not "inbox"

	var titles []string
	for _, s := range cat.Snapshot().Apps[0].Samples {
		titles = append(titles, s.Title)
	}
	if want := "news,inbox"; strings.Join(titles, ",") != want {
		t.Errorf("samples = %v, want %s", titles, want)
	}
}

func TestAppCapEvictsLeastRecent(t *testing.T) {
	cat, clk := newTestCatalog(t, CatalogOptions{MaxApps: 2})
	cat.Observe("old.exe", "t")
	clk.tick()
	cat.Observe("mid.exe", "t")
	clk.tick()
	cat.Observe("new.exe", "t")

	snap := cat.Snapshot()
	if len(snap.Apps) != 2 {
		t.Fatalf("len(Apps) = %d, want the cap of 2", len(snap.Apps))
	}
	for _, a := range snap.Apps {
		if a.Process == "old.exe" {
			t.Errorf("the least recently seen app survived eviction: %v", snap.Apps)
		}
	}
}

// R2.4: the lock screen is a state, not an application. Nothing is added and no
// title is read.
func TestLockScreenIsNotAnApp(t *testing.T) {
	cat, clk := newTestCatalog(t, CatalogOptions{})
	cat.Observe("code.exe", "main.go")
	clk.tick()
	cat.Observe("", "")

	snap := cat.Snapshot()
	if len(snap.Apps) != 1 || snap.Apps[0].Process != "code.exe" {
		t.Errorf("Apps = %+v, want only code.exe", snap.Apps)
	}
	if !snap.Locked {
		t.Error("Locked = false, want true after an empty process")
	}
	if snap.LockedSeen != 1 {
		t.Errorf("LockedSeen = %d, want 1", snap.LockedSeen)
	}

	clk.tick()
	cat.Observe("code.exe", "main.go")
	if cat.Snapshot().Locked {
		t.Error("Locked stayed true after the screen was unlocked")
	}
}

// R2.4: an elevated window's placeholder is listed so the user understands why
// something is missing, but it can never become a rule.
func TestElevatedWindowPlaceholderIsNotConfigurable(t *testing.T) {
	cat, _ := newTestCatalog(t, CatalogOptions{})
	cat.Observe("?unknown", "Administrator: Console")

	snap := cat.Snapshot()
	if len(snap.Apps) != 1 {
		t.Fatalf("len(Apps) = %d, want the placeholder to be listed", len(snap.Apps))
	}
	if snap.Apps[0].Configurable {
		t.Error("the elevated-window placeholder must not be configurable")
	}
}

func TestObserveWithoutTitleStillCountsTheApp(t *testing.T) {
	cat, _ := newTestCatalog(t, CatalogOptions{})
	cat.Observe("notitle.exe", "")

	app := cat.Snapshot().Apps[0]
	if app.Count != 1 {
		t.Errorf("Count = %d, want 1", app.Count)
	}
	if len(app.Samples) != 0 {
		t.Errorf("Samples = %+v, want none for an untitled window", app.Samples)
	}
}

// Handlers serialize a snapshot while the observer keeps writing; the copy must
// be deep or the two race on the sample slice.
func TestSnapshotIsADeepCopy(t *testing.T) {
	cat, _ := newTestCatalog(t, CatalogOptions{})
	cat.Observe("code.exe", "main.go")

	snap := cat.Snapshot()
	snap.Apps[0].Samples[0].Title = "mutated"
	snap.Apps[0].Process = "mutated.exe"

	fresh := cat.Snapshot()
	if fresh.Apps[0].Samples[0].Title != "main.go" || fresh.Apps[0].Process != "code.exe" {
		t.Errorf("mutating a snapshot changed the catalog: %+v", fresh.Apps[0])
	}
}

func TestTitles(t *testing.T) {
	cat, clk := newTestCatalog(t, CatalogOptions{})
	cat.Observe("chrome.exe", "youtube")
	clk.tick()
	cat.Observe("chrome.exe", "github")

	if got := cat.Titles("Chrome.exe"); strings.Join(got, ",") != "github,youtube" {
		t.Errorf("Titles = %v, want most recent first", got)
	}
	if got := cat.Titles("ghost.exe"); got != nil {
		t.Errorf("Titles for an unseen process = %v, want nil", got)
	}
}

// The observer goroutine writes while handlers read; -race must stay clean.
func TestCatalogIsConcurrencySafe(t *testing.T) {
	cat := NewCatalog(CatalogOptions{MaxSamplesPerApp: 4, MaxApps: 8})
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				cat.Observe(fmt.Sprintf("app%d.exe", i%12), fmt.Sprintf("title %d-%d", w, i))
			}
		}(w)
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				snap := cat.Snapshot()
				for _, a := range snap.Apps {
					_ = a.Samples
				}
				cat.Titles("app1.exe")
			}
		}()
	}
	wg.Wait()

	if n := len(cat.Snapshot().Apps); n > 8 {
		t.Errorf("len(Apps) = %d, want the cap of 8 to hold under concurrency", n)
	}
}
