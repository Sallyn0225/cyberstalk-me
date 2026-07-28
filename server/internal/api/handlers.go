package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"cyberstalk.me/server/internal/hub"
	"cyberstalk.me/server/internal/state"
	"cyberstalk.me/server/internal/store"
	"cyberstalk.me/shared"
)

// Handlers holds the dependencies needed by the HTTP handlers.
type Handlers struct {
	store   *store.Store
	hub     *hub.Hub
	tracker *state.Tracker
	now     func() time.Time
}

// New returns Handlers. now is used to stamp last_seen_at (server clock).
func New(s *store.Store, h *hub.Hub, t *state.Tracker, now func() time.Time) *Handlers {
	if now == nil {
		now = time.Now
	}
	return &Handlers{store: s, hub: h, tracker: t, now: now}
}

// Report handles POST /api/v1/report.
//
// It authenticates the bearer token, verifies the device_id in the body
// matches the token, upserts the sanitized state, marks the device online,
// and broadcasts an update event. 500 bodies are always "internal error".
func (h *Handlers) Report(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	dev, err := authFromBearer(ctx, h.store, r)
	if err != nil {
		if errors.Is(err, ErrBadToken) {
			writeError(w, http.StatusUnauthorized, "invalid device token")
			return
		}
		slog.Error("report auth lookup", "err", err)
		writeInternalError(w)
		return
	}

	var p shared.ReportPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "malformed json body")
		return
	}
	// Validate required fields. Missing device_id / device_type / activity
	// is a client bug, not an internal error.
	if strings_isspace(p.DeviceID) {
		writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	if p.DeviceType != "windows" && p.DeviceType != "android" {
		writeError(w, http.StatusBadRequest, "device_type must be windows or android")
		return
	}

	// device_id in body must match the token-bound device.
	if p.DeviceID != dev.DeviceID {
		slog.Warn("report rejected: device_id mismatch", "device_id", p.DeviceID)
		writeError(w, http.StatusUnauthorized, "invalid device token")
		return
	}

	// Use the server clock for last_seen (online judgment). The client's
	// reported_at is only stored for display/debug.
	seenAt := h.now()
	reportedAt := p.ReportedAt
	if reportedAt.IsZero() {
		reportedAt = seenAt
	}
	// Persist the sanitized payload under the trusted device identity. We
	// re-stamp the device identity from the authenticated device so a client
	// can't rename itself or spoof another id in the body.
	p.DeviceID = dev.DeviceID
	p.DeviceName = dev.DeviceName
	p.DeviceType = dev.DeviceType

	payloadJSON, err := json.Marshal(p)
	if err != nil {
		slog.Error("report marshal payload", "err", err)
		writeInternalError(w)
		return
	}
	if err := h.store.UpsertState(ctx, dev.DeviceID, payloadJSON, reportedAt, seenAt); err != nil {
		slog.Error("report upsert state", "device_id", dev.DeviceID, "err", err)
		writeInternalError(w)
		return
	}
	h.tracker.MarkOnline(dev.DeviceID)

	ev := shared.Event{
		Type: "update",
		Device: shared.DeviceState{
			DeviceID:   dev.DeviceID,
			DeviceName: dev.DeviceName,
			DeviceType: dev.DeviceType,
			Activity:   p.Activity,
			Battery:    p.Battery,
			Network:    p.Network,
			Online:     true,
			ReportedAt: reportedAt,
			LastSeenAt: seenAt,
		},
	}
	h.hub.Broadcast(ev)

	slog.Debug("report accepted", "device_id", dev.DeviceID)
	w.WriteHeader(http.StatusNoContent)
}

// Snapshot handles GET /api/v1/snapshot. It returns the current state of
// every device that has reported at least once.
func (h *Handlers) Snapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.store.ListStates(ctx)
	if err != nil {
		slog.Error("snapshot list states", "err", err)
		writeInternalError(w)
		return
	}
	out := make([]shared.DeviceState, 0, len(rows))
	for _, row := range rows {
		out = append(out, shared.DeviceState{
			DeviceID:   row.DeviceID,
			DeviceName: row.DeviceName,
			DeviceType: row.DeviceType,
			Activity:   row.Payload.Activity,
			Battery:    row.Payload.Battery,
			Network:    row.Payload.Network,
			Online:     h.tracker.IsOnline(row.LastSeenAt),
			ReportedAt: row.ReportedAt,
			LastSeenAt: row.LastSeenAt,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Stream handles GET /api/v1/stream (Server-Sent Events). It subscribes to
// the hub and writes each event as an SSE message. On client disconnect or
// handler return, it unsubscribes so the hub can drop the channel.
func (h *Handlers) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: don't buffer SSE

	ch := make(chan shared.Event, 16)
	unsubscribe := h.hub.Subscribe(ch)
	defer unsubscribe()

	// Send an initial hello so the client knows the stream is alive; the
	// real data comes from snapshot + subsequent events.
	fmt.Fprintf(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				// Hub closed the channel (slow subscriber / shutdown).
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				slog.Error("stream marshal event", "err", err)
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				// Write error means the subscriber is gone. Returning
				// triggers the deferred unsubscribe.
				return
			}
			flusher.Flush()
		}
	}
}
