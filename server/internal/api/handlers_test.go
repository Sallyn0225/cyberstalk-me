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
	h, clock, _ := setupWithDB(t)
	return h, clock
}

// testTZ is the display timezone the handlers use in tests. A fixed +08:00
// offset rather than time.LoadLocation("Asia/Shanghai") so the tests do not
// need a tz database; the offset is all the window math uses.
var testTZ = time.FixedZone("UTC+8", 8*3600)

// testMaxGap is the attribution gap limit the handlers get in tests.
const testMaxGap = 60 * time.Second

// setupWithDB is setup plus the raw *sql.DB, for tests that break the
// database on purpose (the store itself is never mocked).
func setupWithDB(t *testing.T) (*Handlers, *fakeClock, *sql.DB) {
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
	handlers := New(st, h, tracker, clock.now, testMaxGap, testTZ)
	return handlers, clock, db
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

func TestUsageRejectsUnknownWindow(t *testing.T) {
	h, _ := setup(t)
	for _, window := range []string{"1d", "yesterday", "TODAY", "7"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/usage?window="+window, nil)
		rec := httptest.NewRecorder()
		h.Usage(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("window %q: want 400, got %d", window, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "window must be one of") {
			t.Fatalf("window %q: unexpected body %s", window, rec.Body.String())
		}
	}
}

func TestUsageDefaultsToToday(t *testing.T) {
	h, _ := setup(t)
	resp := getUsage(t, h, "")
	if resp.Window != "today" {
		t.Fatalf("window = %q, want today", resp.Window)
	}
	if resp.Timezone != testTZ.String() {
		t.Fatalf("timezone = %q, want %q", resp.Timezone, testTZ.String())
	}
	// from is local midnight of the fake clock's day: 2026-07-28 00:00 +08:00.
	wantFrom := time.Date(2026, 7, 28, 0, 0, 0, 0, testTZ)
	if !resp.From.Equal(wantFrom) {
		t.Fatalf("from = %s, want %s", resp.From, wantFrom)
	}
	if resp.Devices == nil || len(resp.Devices) != 0 {
		t.Fatalf("devices = %+v, want an empty non-nil slice", resp.Devices)
	}
}

func TestUsageWindowsCoverWholeLocalDays(t *testing.T) {
	h, _ := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")
	postReport(t, h, "tok-1", report("Code", "写代码"))

	tests := []struct {
		window   string
		wantFrom time.Time
		wantDays int
	}{
		// Seven local days including today, not the last 168 hours. The 30d
		// window also has to cross a month boundary correctly.
		{"7d", time.Date(2026, 7, 22, 0, 0, 0, 0, testTZ), 7},
		{"30d", time.Date(2026, 6, 29, 0, 0, 0, 0, testTZ), 30},
	}
	for _, tt := range tests {
		resp := getUsage(t, h, tt.window)
		if !resp.From.Equal(tt.wantFrom) {
			t.Errorf("window %s: from = %s, want %s", tt.window, resp.From, tt.wantFrom)
		}
		dev := onlyDevice(t, resp)
		if len(dev.Daily) != tt.wantDays {
			t.Errorf("window %s: %d day slots, want %d", tt.window, len(dev.Daily), tt.wantDays)
		}
		if dev.Hourly != nil {
			t.Errorf("window %s: hourly = %+v, want nil", tt.window, dev.Hourly)
		}
		if len(dev.Daily) > 0 && dev.Daily[len(dev.Daily)-1].Date != "2026-07-28" {
			t.Errorf("window %s: last day = %q, want 2026-07-28", tt.window, dev.Daily[len(dev.Daily)-1].Date)
		}
	}
}

// TestUsageCreditsPreviousReportsActivity is the core attribution rule: the
// interval between two reports belongs to what the earlier report described.
func TestUsageCreditsPreviousReportsActivity(t *testing.T) {
	h, clock := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	postReport(t, h, "tok-1", report("Code", "写代码"))
	clock.advance(30 * time.Second)
	postReport(t, h, "tok-1", report("Chrome", "上网"))

	resp := getUsage(t, h, "today")
	dev := onlyDevice(t, resp)
	if dev.Totals.ActiveSeconds != 30 {
		t.Fatalf("active total = %d, want 30", dev.Totals.ActiveSeconds)
	}
	// Chrome has not been observed for any completed interval yet, so it must
	// not appear at all — crediting the new report would give it the 30s.
	if len(dev.Apps) != 1 || dev.Apps[0].App != "Code" || dev.Apps[0].Seconds != 30 {
		t.Fatalf("apps = %+v, want only Code with 30s", dev.Apps)
	}
	if len(dev.Apps[0].Activities) != 1 || dev.Apps[0].Activities[0].Description != "写代码" ||
		dev.Apps[0].Activities[0].Seconds != 30 {
		t.Fatalf("activities = %+v, want 写代码 with 30s", dev.Apps[0].Activities)
	}

	// A third report closes Chrome's first interval.
	clock.advance(10 * time.Second)
	postReport(t, h, "tok-1", report("Chrome", "上网"))
	resp = getUsage(t, h, "today")
	dev = onlyDevice(t, resp)
	if dev.Totals.ActiveSeconds != 40 {
		t.Fatalf("active total = %d, want 40", dev.Totals.ActiveSeconds)
	}
	if seconds, ok := appSeconds(dev, "Chrome"); !ok || seconds != 10 {
		t.Fatalf("Chrome = %d (found %v), want 10", seconds, ok)
	}
}

func TestUsageIdleTimeIsNotAppTime(t *testing.T) {
	h, clock := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	idle := report("Code", "写代码")
	idle.Activity.Idle = true
	idle.Activity.IdleSeconds = 400
	postReport(t, h, "tok-1", idle)
	clock.advance(30 * time.Second)
	postReport(t, h, "tok-1", report("Code", "写代码"))

	dev := onlyDevice(t, getUsage(t, h, "today"))
	if dev.Totals.IdleSeconds != 30 {
		t.Fatalf("idle total = %d, want 30", dev.Totals.IdleSeconds)
	}
	if dev.Totals.ActiveSeconds != 0 {
		t.Fatalf("active total = %d, want 0", dev.Totals.ActiveSeconds)
	}
	if len(dev.Apps) != 0 {
		t.Fatalf("apps = %+v, want empty: idle time is not app time", dev.Apps)
	}
}

func TestUsageLockedTimeIsNeitherAppNorIdleTime(t *testing.T) {
	h, clock := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	// The realistic lock-screen report: Windows stops advancing the input
	// timer while locked, so idle is false and only locked marks it.
	locked := report("锁屏", "屏幕已锁定")
	locked.Activity.Locked = true
	postReport(t, h, "tok-1", locked)
	clock.advance(30 * time.Second)
	postReport(t, h, "tok-1", report("Code", "写代码"))

	dev := onlyDevice(t, getUsage(t, h, "today"))
	if dev.Totals.LockedSeconds != 30 {
		t.Fatalf("locked total = %d, want 30", dev.Totals.LockedSeconds)
	}
	if dev.Totals.IdleSeconds != 0 || dev.Totals.ActiveSeconds != 0 {
		t.Fatalf("totals = %+v, want locked only", dev.Totals)
	}
	if len(dev.Apps) != 0 {
		t.Fatalf("apps = %+v, want empty: the locked placeholder app must not rank", dev.Apps)
	}
}

func TestUsageDropsIntervalsLongerThanMaxGap(t *testing.T) {
	h, clock := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	postReport(t, h, "tok-1", report("Code", "写代码"))
	// Silent for far longer than testMaxGap: we do not know what the device
	// was doing, so none of it may be attributed.
	clock.advance(4 * time.Hour)
	postReport(t, h, "tok-1", report("Code", "写代码"))

	dev := onlyDevice(t, getUsage(t, h, "today"))
	if dev.Totals != (shared.UsageTotals{}) {
		t.Fatalf("totals = %+v, want all zeros", dev.Totals)
	}
	if len(dev.Apps) != 0 {
		t.Fatalf("apps = %+v, want empty", dev.Apps)
	}

	// Reporting normally again resumes attribution.
	clock.advance(20 * time.Second)
	postReport(t, h, "tok-1", report("Code", "写代码"))
	dev = onlyDevice(t, getUsage(t, h, "today"))
	if dev.Totals.ActiveSeconds != 20 {
		t.Fatalf("active total = %d, want 20 after the gap", dev.Totals.ActiveSeconds)
	}
}

func TestUsageSplitsIntervalAcrossHourBoundary(t *testing.T) {
	h, clock := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	// Move to 10:59:55 UTC, which is 18:59:55 in the +08:00 display zone.
	clock.advance(59*time.Minute + 55*time.Second)
	postReport(t, h, "tok-1", report("Code", "写代码"))
	clock.advance(10 * time.Second)
	postReport(t, h, "tok-1", report("Code", "写代码"))

	dev := onlyDevice(t, getUsage(t, h, "today"))
	if dev.Totals.ActiveSeconds != 10 {
		t.Fatalf("active total = %d, want 10", dev.Totals.ActiveSeconds)
	}
	if len(dev.Hourly) != 24 {
		t.Fatalf("hourly has %d slots, want 24", len(dev.Hourly))
	}
	if dev.Hourly[18].Seconds != 5 || dev.Hourly[19].Seconds != 5 {
		t.Fatalf("hours 18/19 = %d/%d, want 5/5", dev.Hourly[18].Seconds, dev.Hourly[19].Seconds)
	}
	if dev.Hourly[18].TopApp != "Code" || dev.Hourly[19].TopApp != "Code" {
		t.Fatalf("top apps = %q/%q, want Code/Code", dev.Hourly[18].TopApp, dev.Hourly[19].TopApp)
	}
	if dev.Daily != nil {
		t.Fatalf("daily = %+v, want nil for window today", dev.Daily)
	}
}

// TestReportSurvivesUsageWriteFailure is the guarantee that statistics are
// secondary: dropping the table breaks only the usage write, and the report
// must still be accepted and broadcast.
func TestReportSurvivesUsageWriteFailure(t *testing.T) {
	h, clock, db := setupWithDB(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	// First report establishes a previous interval so attribution runs at all.
	postReport(t, h, "tok-1", report("Code", "写代码"))

	if _, err := db.ExecContext(context.Background(), `DROP TABLE usage_bucket`); err != nil {
		t.Fatalf("drop usage_bucket: %v", err)
	}

	events := make(chan shared.Event, 4)
	unsubscribe := h.hub.Subscribe(events)
	defer unsubscribe()

	clock.advance(30 * time.Second)
	body := mustMarshal(t, report("Code", "写代码"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok-1")
	rec := httptest.NewRecorder()
	h.Report(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204 despite the broken usage table, got %d %s", rec.Code, rec.Body.String())
	}
	select {
	case ev := <-events:
		if ev.Type != "update" || ev.Device.DeviceID != "d1" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	default:
		t.Fatal("no event broadcast")
	}

	// The state upsert must have gone through too, not just the response code.
	row, err := h.store.GetState(context.Background(), "d1")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if !row.LastSeenAt.Equal(clock.t) {
		t.Fatalf("last_seen_at = %s, want %s", row.LastSeenAt, clock.t)
	}
}

// TestUsageAfterPruneReturnsZeros covers the retention path end to end: once
// the buckets are gone the endpoint still answers, with zeros.
func TestUsageAfterPruneReturnsZeros(t *testing.T) {
	h, clock := setup(t)
	registerDevice(t, h, "d1", "PC", "windows", "tok-1")

	postReport(t, h, "tok-1", report("Code", "写代码"))
	clock.advance(30 * time.Second)
	postReport(t, h, "tok-1", report("Code", "写代码"))
	if dev := onlyDevice(t, getUsage(t, h, "today")); dev.Totals.ActiveSeconds != 30 {
		t.Fatalf("active total = %d before pruning, want 30", dev.Totals.ActiveSeconds)
	}

	// A retention window of zero days: everything before now goes.
	n, err := h.store.PruneUsage(context.Background(), clock.now())
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n == 0 {
		t.Fatal("prune removed nothing")
	}

	resp := getUsage(t, h, "today")
	dev := onlyDevice(t, resp)
	if dev.Totals != (shared.UsageTotals{}) {
		t.Fatalf("totals = %+v after pruning, want zeros", dev.Totals)
	}
	// The device still shows up — it has reported, it just has no data left.
	if dev.DeviceID != "d1" || len(dev.Hourly) != 24 {
		t.Fatalf("unexpected device after pruning: %+v", dev)
	}
}

// report builds a minimal valid report payload for device d1.
func report(app, description string) shared.ReportPayload {
	return shared.ReportPayload{
		DeviceID:   "d1",
		DeviceType: "windows",
		Activity:   shared.Activity{App: app, Description: description},
	}
}

// getUsage GETs /api/v1/usage and decodes the response. An empty window omits
// the query parameter entirely.
func getUsage(t *testing.T, h *Handlers, window string) shared.UsageResponse {
	t.Helper()
	url := "/api/v1/usage"
	if window != "" {
		url += "?window=" + window
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	h.Usage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("usage: want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var resp shared.UsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	return resp
}

// onlyDevice asserts the response has exactly one device and returns it.
func onlyDevice(t *testing.T, resp shared.UsageResponse) shared.DeviceUsage {
	t.Helper()
	if len(resp.Devices) != 1 {
		t.Fatalf("got %d devices, want 1: %+v", len(resp.Devices), resp.Devices)
	}
	return resp.Devices[0]
}

func appSeconds(dev shared.DeviceUsage, app string) (int, bool) {
	for _, a := range dev.Apps {
		if a.App == app {
			return a.Seconds, true
		}
	}
	return 0, false
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
