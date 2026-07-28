// Package hub is the in-memory SSE broadcast hub.
//
// It maintains a set of subscriber channels and fans out Events to all of
// them. The hub owns its subscriber map behind a mutex so the race detector
// stays clean. Fan-out is non-blocking: a full or closed subscriber channel
// means the subscriber is gone/stale and is removed (unsubscribed) rather
// than blocking the broadcaster.
package hub

import (
	"log/slog"
	"sync"

	"cyberstalk.me/shared"
)

// Hub fans out Events to all subscribed channels.
type Hub struct {
	mu   sync.Mutex
	subs map[chan shared.Event]struct{}
}

// New returns an empty hub.
func New() *Hub {
	return &Hub{subs: make(map[chan shared.Event]struct{})}
}

// Subscribe registers ch to receive broadcast events and returns a
// function to unsubscribe (safe to call multiple times). The caller owns ch
// and should also call Unsubscribe when done.
func (h *Hub) Subscribe(ch chan shared.Event) func() {
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	slog.Debug("sse subscribe", "subscribers", h.count())
	return func() {
		h.Unsubscribe(ch)
	}
}

// Unsubscribe removes ch from the subscriber set if present. Safe to call
// concurrently with Broadcast.
func (h *Hub) Unsubscribe(ch chan shared.Event) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
	slog.Debug("sse unsubscribe", "subscribers", h.count())
}

// Broadcast sends ev to every subscriber. Subscribers whose channel is not
// ready (full or closed) are removed. This never blocks the caller.
func (h *Hub) Broadcast(ev shared.Event) {
	h.mu.Lock()
	dead := make([]chan shared.Event, 0)
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			dead = append(dead, ch)
		}
	}
	for _, ch := range dead {
		delete(h.subs, ch)
		close(ch)
	}
	n := len(h.subs)
	h.mu.Unlock()
	if len(dead) > 0 {
		slog.Debug("sse dropped slow subscribers", "dropped", len(dead), "subscribers", n)
	}
}

// count must be called with the mutex held.
func (h *Hub) count() int {
	return len(h.subs)
}
