package state

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"cyberstalk.me/server/internal/hub"
	"cyberstalk.me/server/internal/store"
	"cyberstalk.me/shared"

	_ "modernc.org/sqlite" // register sqlite driver for tests
)

func newTrackerStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	s, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatalf("init store: %v", err)
	}
	return s
}

func TestTrackerOfflineTransitionBroadcasts(t *testing.T) {
	s := newTrackerStore(t)
	ctx := context.Background()

	hash := store.HashToken("tok")
	if err := s.RegisterDevice(ctx, "d1", "PC", "windows", hash, time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}
	// A stale report: last_seen_at well in the past relative to clock start.
	clock := &fakeClock{t: time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)}
	staleSeen := clock.t.Add(-5 * time.Minute)
	p := shared.ReportPayload{DeviceID: "d1", DeviceType: "windows", Activity: shared.Activity{App: "a"}}
	payload, _ := jsonMarshal(p)
	if err := s.UpsertState(ctx, "d1", payload, staleSeen, staleSeen); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	h := hub.New()
	tr := New(s, h, 60*time.Second, 5*time.Second, clock.now)
	// Pretend it was known-online before so the scan sees a transition.
	tr.MarkOnline("d1")

	// Subscribe to the hub to observe the offline event.
	ch := make(chan shared.Event, 4)
	h.Subscribe(ch)

	tr.scanOnce(ctx)

	select {
	case ev := <-ch:
		if ev.Type != "offline" {
			t.Fatalf("want offline event, got %q", ev.Type)
		}
		if ev.Device.DeviceID != "d1" {
			t.Fatalf("want d1, got %s", ev.Device.DeviceID)
		}
		if ev.Device.Online {
			t.Fatalf("device should be offline")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not receive offline event")
	}
}

func TestTrackerIsOnline(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)}
	tr := New(nil, nil, 60*time.Second, 5*time.Second, clock.now)
	if !tr.IsOnline(clock.t.Add(-30 * time.Second)) {
		t.Fatalf("30s ago should be online")
	}
	if tr.IsOnline(clock.t.Add(-70 * time.Second)) {
		t.Fatalf("70s ago should be offline")
	}
}

// fakeClock for the tracker tests.
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) now() time.Time { return f.t }

func jsonMarshal(p shared.ReportPayload) ([]byte, error) {
	return jsonMarshalImpl(p)
}

// TestTrackerConcurrentMarkOnlineAndScan hammers MarkOnline and scanOnce
// from many goroutines at once. Without the mutex guarding the known map
// this fatals with "concurrent map writes" (and go test -race would flag a
// data race). It lets environments without a C compiler (so no -race) still
// catch the bug at runtime.
func TestTrackerConcurrentMarkOnlineAndScan(t *testing.T) {
	s := newTrackerStore(t)
	ctx := context.Background()

	for _, id := range []string{"d1", "d2"} {
		if err := s.RegisterDevice(ctx, id, "PC", "windows", store.HashToken("tok-"+id), time.Now().UTC()); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
		p := shared.ReportPayload{DeviceID: id, DeviceType: "windows", Activity: shared.Activity{App: "a"}}
		payload, err := jsonMarshal(p)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := s.UpsertState(ctx, id, payload, time.Now().UTC(), time.Now().UTC()); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}

	clock := &fakeClock{t: time.Now().UTC()}
	h := hub.New()
	tr := New(s, h, 60*time.Second, 5*time.Second, clock.now)

	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "d1"
			if n%2 == 0 {
				id = "d2"
			}
			tr.MarkOnline(id)
			tr.scanOnce(ctx)
		}(i)
	}
	wg.Wait()
}
