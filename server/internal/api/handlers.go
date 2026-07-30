package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"cyberstalk.me/server/internal/hub"
	"cyberstalk.me/server/internal/state"
	"cyberstalk.me/server/internal/store"
	"cyberstalk.me/server/internal/usage"
	"cyberstalk.me/shared"
)

// Handlers holds the dependencies needed by the HTTP handlers.
type Handlers struct {
	store   *store.Store
	hub     *hub.Hub
	tracker *state.Tracker
	now     func() time.Time
	maxGap  time.Duration
	loc     *time.Location
}

// New returns Handlers. now is used to stamp last_seen_at (server clock).
// maxGap must be positive — config.Load is what guarantees that; a zero would
// discard every attribution interval as a gap. loc is the site timezone the
// usage windows are computed in; nil falls back to UTC.
func New(s *store.Store, h *hub.Hub, t *state.Tracker, now func() time.Time, maxGap time.Duration, loc *time.Location) *Handlers {
	if now == nil {
		now = time.Now
	}
	if loc == nil {
		loc = time.UTC
	}
	return &Handlers{store: s, hub: h, tracker: t, now: now, maxGap: maxGap, loc: loc}
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

	// Credit the time since the previous report to the activity that report
	// described — the device was doing that, not this, for the interval that
	// just ended. This has to run before UpsertState, while the stored state
	// is still the previous observation.
	//
	// Nothing in here may fail the request: usage is a secondary aggregate,
	// and losing an hour of statistics is not a reason to stop accepting
	// reports or to let the device think it is offline.
	h.attributeUsage(ctx, dev.DeviceID, seenAt)

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

// attributeUsage accumulates the interval between the device's previous
// report and seenAt into the hourly usage buckets. Every failure path only
// logs: the caller's response must not change because of it.
//
// Only the server clock is used. The client's reported_at may be wrong or may
// jump, while last_seen_at is already the basis for the online judgment.
func (h *Handlers) attributeUsage(ctx context.Context, deviceID string, seenAt time.Time) {
	prev, err := h.store.GetState(ctx, deviceID)
	if err != nil {
		slog.Error("report read previous state", "device_id", deviceID, "err", err)
		return
	}
	// A registered device that has never reported comes back as a zero-valued
	// row with a nil error, so the absence of a previous interval shows up
	// here and not as an error. First report attributes nothing.
	if prev.LastSeenAt.IsZero() {
		return
	}
	buckets := usage.Attribute(prev.Payload.Activity, prev.LastSeenAt, seenAt, h.maxGap)
	if len(buckets) == 0 {
		return
	}
	deltas := make([]store.UsageDelta, len(buckets))
	for i, b := range buckets {
		deltas[i] = store.UsageDelta{
			HourStart:   b.HourStart,
			State:       b.State,
			App:         b.App,
			Description: b.Description,
			Seconds:     b.Seconds,
		}
	}
	if err := h.store.AddUsage(ctx, deviceID, deltas); err != nil {
		slog.Error("report add usage", "device_id", deviceID, "err", err)
	}
}

// Usage handles GET /api/v1/usage?window=today|7d|30d. It is public and
// read-only, like Snapshot.
//
// A missing window means "today"; an unknown one is a 400 rather than a silent
// fallback, so a typo in a bookmarked URL is visible instead of quietly
// showing the wrong range.
func (h *Handlers) Usage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	window := r.URL.Query().Get("window")
	if window == "" {
		window = usage.WindowToday
	}
	days, ok := usage.WindowDays(window)
	if !ok {
		writeError(w, http.StatusBadRequest, "window must be one of today, 7d, 30d")
		return
	}

	// The window is whole local days including today, not the last N*24
	// hours: the daily chart wants complete day slots. time.Date normalizes an
	// out-of-range day, so month and year boundaries need no special case.
	to := h.now()
	local := to.In(h.loc)
	from := time.Date(local.Year(), local.Month(), local.Day()-(days-1), 0, 0, 0, 0, h.loc)

	// Every device that has reported at least once appears, even with no
	// buckets in the window; devices that never reported stay out, matching
	// Snapshot.
	states, err := h.store.ListStates(ctx)
	if err != nil {
		slog.Error("usage list states", "err", err)
		writeInternalError(w)
		return
	}
	devices := make([]usage.Device, len(states))
	for i, s := range states {
		devices[i] = usage.Device{
			DeviceID:   s.DeviceID,
			DeviceName: s.DeviceName,
			DeviceType: s.DeviceType,
		}
	}

	stored, err := h.store.QueryUsage(ctx, from.UTC(), to.UTC())
	if err != nil {
		slog.Error("usage query buckets", "err", err)
		writeInternalError(w)
		return
	}
	rows := make([]usage.Row, len(stored))
	for i, s := range stored {
		rows[i] = usage.Row{
			DeviceID:    s.DeviceID,
			HourStart:   s.HourStart,
			State:       s.State,
			App:         s.App,
			Description: s.Description,
			Seconds:     s.Seconds,
		}
	}

	resp := usage.Aggregate(devices, rows, window, from.UTC(), to.UTC(), h.loc)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
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
