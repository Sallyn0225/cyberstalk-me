package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cyberstalk.me/server/internal/hub"
	"cyberstalk.me/server/internal/state"
	"cyberstalk.me/server/internal/store"
	"cyberstalk.me/shared"

	_ "modernc.org/sqlite" // register sqlite driver for tests
)

// setup builds a Handlers backed by an in-memory SQLite store and a hub,
// with a fake clock that starts at a fixed time and only advances when the
// test calls h.advance. This lets the offline-threshold transition test
// run instantly without sleeping.
func setup(t *testing.T) (*Handlers, *fakeClock) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	st, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatalf("init store: %v", err)
	}
	clock := &fakeClock{t: time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)}
	h := hub.New()
	tracker := state.New(st, h, 60*time.Second, 5*time.Second, clock.now)
	handlers := New(st, h, tracker, clock.now)
	return handlers, clock
}

type fakeClock struct {
	t time.Time
}

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

// registerDevice registers a device via the store with a known token and
// returns the raw token to use in Authorization headers.
func registerDevice(t *testing.T, h *Handlers, id, name, dtype, rawToken string) {
	t.Helper()
	hash := store.HashToken(rawToken)
	if err := h.store.RegisterDevice(context.Background(), id, name, dtype, hash, time.Now().UTC()); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
}

func TestReportMissingTokenRejected(t *testing.T) {
	h, _ := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	body := mustMarshal(t, shared.ReportPayload{DeviceID: "d1", DeviceType: "windows", Activity: shared.Activity{App: "a"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Report(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid device token") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestReportWrongTokenRejected(t *testing.T) {
	h, _ := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	body := mustMarshal(t, shared.ReportPayload{DeviceID: "d1", DeviceType: "windows", Activity: shared.Activity{App: "a"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	h.Report(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestReportDeviceIDMismatchRejected(t *testing.T) {
	h, _ := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")
	registerDevice(t, h, "d2", "Laptop", "windows", "tok-2")

	// d2's token claims to be d1 in the body — must be rejected.
	body := mustMarshal(t, shared.ReportPayload{DeviceID: "d1", DeviceType: "windows", Activity: shared.Activity{App: "a"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok-2")
	rec := httptest.NewRecorder()
	h.Report(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for device_id mismatch, got %d", rec.Code)
	}
}

func TestReportBadDeviceTypeRejected(t *testing.T) {
	h, _ := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	body := mustMarshal(t, shared.ReportPayload{DeviceID: "d1", DeviceType: "iphone", Activity: shared.Activity{App: "a"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok-1")
	rec := httptest.NewRecorder()
	h.Report(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad device_type, got %d", rec.Code)
	}
}

func TestReportThenSnapshot(t *testing.T) {
	h, _ := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")
	registerDevice(t, h, "d2", "Phone", "android", "tok-2")

	net := "wifi"
	p1 := shared.ReportPayload{DeviceID: "d1", DeviceName: "PC", DeviceType: "windows", Activity: shared.Activity{App: "VS Code", Description: "writing code"}, Battery: &shared.Battery{Level: ptrInt(82), Charging: true}, Network: &net, ReportedAt: time.Now().UTC()}
	postReport(t, h, "tok-1", p1)
	p2 := shared.ReportPayload{DeviceID: "d2", DeviceName: "Phone", DeviceType: "android", Activity: shared.Activity{App: "Chrome", Description: "browsing"}, Network: &net, ReportedAt: time.Now().UTC()}
	postReport(t, h, "tok-2", p2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot", nil)
	rec := httptest.NewRecorder()
	h.Snapshot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var states []shared.DeviceState
	if err := json.Unmarshal(rec.Body.Bytes(), &states); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("want 2 devices, got %d", len(states))
	}
	// Both should be online right after reporting.
	for _, s := range states {
		if !s.Online {
			t.Fatalf("device %s should be online", s.DeviceID)
		}
	}
}

func TestOfflineTransitionAfterThreshold(t *testing.T) {
	h, clock := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	net := "wifi"
	postReport(t, h, "tok-1", shared.ReportPayload{DeviceID: "d1", DeviceName: "PC", DeviceType: "windows", Activity: shared.Activity{App: "VS Code"}, Network: &net, ReportedAt: clock.now()})

	// Device is online now.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot", nil)
	rec := httptest.NewRecorder()
	h.Snapshot(rec, req)
	var states []shared.DeviceState
	if err := json.Unmarshal(rec.Body.Bytes(), &states); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !states[0].Online {
		t.Fatalf("expected online immediately after report")
	}

	// Advance past the threshold (60s). Snapshot must now show offline.
	clock.advance(90 * time.Second)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/snapshot", nil)
	rec = httptest.NewRecorder()
	h.Snapshot(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &states); err != nil {
		t.Fatalf("decode 2: %v", err)
	}
	if states[0].Online {
		t.Fatalf("expected offline after threshold, last_seen=%s now=%s", states[0].LastSeenAt, clock.now())
	}
	if states[0].LastSeenAt.IsZero() {
		t.Fatalf("last_seen_at should be populated")
	}
}

func TestSnapshotEmpty(t *testing.T) {
	h, _ := setup(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot", nil)
	rec := httptest.NewRecorder()
	h.Snapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "[]") && rec.Body.String() != "[]\n" {
		t.Fatalf("expected empty array, got %s", rec.Body.String())
	}
}

// postReport is a helper that POSTs a report with the given bearer token.
func postReport(t *testing.T, h *Handlers, token string, p shared.ReportPayload) {
	t.Helper()
	body := mustMarshal(t, p)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.Report(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("report failed: %d %s", rec.Code, rec.Body.String())
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func ptrInt(v int) *int { return &v }
