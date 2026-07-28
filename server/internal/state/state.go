// Package state tracks device online/offline status.
//
// Online judgment uses the server clock: when a report arrives, main stamps
// last_seen_at; the tracker compares that to now() periodically. A device
// whose last_seen_at is older than the offline threshold is judged offline,
// and the transition (online->offline) is broadcast to the hub. The tracker
// accepts an injectable now() so tests can move time forward without
// sleeping.
//
// This package depends only on shared, hub, and the StateLister interface
// below — never on the store package. That keeps hub/state/store mutually
// decoupled per directory-structure.md (the only allowed cross-dep among
// them is state -> hub for offline broadcasts). main.go wires *store.Store
// in as the StateLister implementation.
package state

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"cyberstalk.me/server/internal/hub"
	"cyberstalk.me/shared"
)

// StateLister returns the latest projection of every device that has
// reported at least once. The Online field of each DeviceState is not
// populated by the lister (it has no clock); the tracker judges online
// status itself from LastSeenAt. *store.Store satisfies this interface.
type StateLister interface {
	ListDeviceStates(ctx context.Context) ([]shared.DeviceState, error)
}

// Tracker scans the store periodically and marks devices offline when their
// last_seen_at exceeds the threshold. It is the only writer of the "online"
// judgment (handlers compute it on demand for snapshots using IsOnline).
type Tracker struct {
	lister    StateLister
	hub       *hub.Hub
	threshold time.Duration
	scan      time.Duration
	now       func() time.Time

	// mu guards known. known is written from HTTP handler goroutines
	// (MarkOnline, on every report) and read/written from the tracker
	// goroutine (scanOnce). Without this lock the race detector flags a
	// data race and the runtime can fatal with "concurrent map writes".
	mu    sync.Mutex
	known map[string]struct{}
}

// New returns a tracker. now defaults to time.Now; tests may inject a fake
// clock. scan and threshold come from config. lister is satisfied by
// *store.Store (wired in main); nil is allowed for tests that only exercise
// IsOnline.
func New(l StateLister, h *hub.Hub, threshold, scan time.Duration, now func() time.Time) *Tracker {
	if now == nil {
		now = time.Now
	}
	return &Tracker{
		lister:    l,
		hub:       h,
		threshold: threshold,
		scan:      scan,
		now:       now,
		known:     make(map[string]struct{}),
	}
}

// IsOnline returns whether a device with the given last_seen_at is online
// according to the threshold and the provided clock. It is a pure helper
// also used by handlers building snapshot responses.
func (t *Tracker) IsOnline(lastSeen time.Time) bool {
	return t.now().Sub(lastSeen) <= t.threshold
}

// Run scans for newly-offline devices every scan interval until ctx is
// cancelled. It must run in its own goroutine.
func (t *Tracker) Run(ctx context.Context) {
	ticker := time.NewTicker(t.scan)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.scanOnce(ctx)
		}
	}
}

// scanOnce performs a single offline scan. It lists all states, finds
// devices that flipped from online to offline, and broadcasts offline
// events for them.
func (t *Tracker) scanOnce(ctx context.Context) {
	states, err := t.lister.ListDeviceStates(ctx)
	if err != nil {
		slog.Error("state scan list states", "err", err)
		return
	}
	now := t.now()
	online := make(map[string]struct{}, len(states))
	var transitions []shared.Event
	for _, ds := range states {
		if now.Sub(ds.LastSeenAt) <= t.threshold {
			online[ds.DeviceID] = struct{}{}
			continue
		}
		// Device is offline now. Only broadcast the transition. The
		// known-set read/delete is under the lock; the broadcast is done
		// after, so we never hold mu while the hub takes its own lock.
		t.mu.Lock()
		_, was := t.known[ds.DeviceID]
		if was {
			delete(t.known, ds.DeviceID)
		}
		t.mu.Unlock()
		if was {
			slog.Info("device offline", "device_id", ds.DeviceID, "last_seen", ds.LastSeenAt)
			ds.Online = false
			transitions = append(transitions, shared.Event{
				Type:   "offline",
				Device: ds,
			})
		}
	}
	// New online devices are recorded when their reports arrive (handler
	// calls MarkOnline), but keep the map in sync in case a report arrived
	// out-of-band or a device re-reported while we were scanning.
	t.mu.Lock()
	for id := range online {
		t.known[id] = struct{}{}
	}
	t.mu.Unlock()

	for _, ev := range transitions {
		t.hub.Broadcast(ev)
	}
}

// MarkOnline records that a device is online (after a report was received).
// The next offline scan will broadcast the transition if it goes silent.
func (t *Tracker) MarkOnline(id string) {
	t.mu.Lock()
	t.known[id] = struct{}{}
	t.mu.Unlock()
}
