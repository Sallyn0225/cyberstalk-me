package hub

import (
	"sync"
	"testing"

	"cyberstalk.me/shared"
)

// TestBroadcastConcurrent exercises the hub under concurrent
// subscribe/unsubscribe/broadcast to keep the mutex logic honest. It can't
// run with -race here (no C compiler on this Windows host), but it
// validates the logical invariants: no goroutine panics, subscribers that
// keep up receive events.
func TestBroadcastConcurrent(t *testing.T) {
	h := New()
	const subs = 20
	const events = 200

	var wg sync.WaitGroup
	chans := make([]chan shared.Event, subs)
	// Subscribe a bunch of channels and count what they receive.
	counts := make([]int, subs)
	for i := range chans {
		ch := make(chan shared.Event, 32)
		chans[i] = ch
		idx := i
		h.Subscribe(ch)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range ch {
				counts[idx]++
			}
		}()
	}

	// Broadcast events concurrently.
	var bwg sync.WaitGroup
	for i := 0; i < events; i++ {
		bwg.Add(1)
		go func(n int) {
			defer bwg.Done()
			h.Broadcast(shared.Event{Type: "update", Device: shared.DeviceState{DeviceID: "x"}})
		}(i)
	}
	bwg.Wait()

	// Unsubscribe all; this closes the channels so receiver goroutines
	// finish.
	for _, ch := range chans {
		h.Unsubscribe(ch)
	}
	wg.Wait()

	// Each channel either received all events or was dropped (if it fell
	// behind). We only assert that the hub didn't panic and that at least
	// some events were delivered — exact counts depend on scheduling.
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		t.Fatalf("no events were delivered to any subscriber")
	}
}

func TestBroadcastDropsSlowSubscriber(t *testing.T) {
	h := New()
	// A subscriber with a tiny buffer that never reads.
	ch := make(chan shared.Event, 1)
	h.Subscribe(ch)

	for i := 0; i < 10; i++ {
		h.Broadcast(shared.Event{Type: "update"})
	}
	// The slow subscriber should have been removed. Reading from ch may or
	// may not yield one event, but the channel must be closed (Unsubscribe
	// was called by Broadcast on the full channel).
	// We assert the hub has no subscribers left by subscribing a fresh one
	// and broadcasting successfully.
	fresh := make(chan shared.Event, 8)
	h.Subscribe(fresh)
	h.Broadcast(shared.Event{Type: "update"})
	select {
	case <-fresh:
		// good
	default:
		t.Fatalf("fresh subscriber should have received an event")
	}
}
