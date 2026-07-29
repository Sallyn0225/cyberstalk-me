package report

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cyberstalk.me/shared"
)

func fixedPayload() shared.ReportPayload {
	return shared.ReportPayload{
		DeviceID:   "win-desktop",
		DeviceType: "windows",
		Activity:   shared.Activity{App: "VS Code", Description: "在写代码"},
	}
}

// recorder is a race-safe capture of the first request the loop sends, plus a
// cycling status sequence for the rest.
type recorder struct {
	mu       sync.Mutex
	count    int
	captured bool
	auth     string
	ct       string
	body     []byte
}

func (r *recorder) handler(statuses []int) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		i := r.count
		if i >= len(statuses) {
			i = len(statuses) - 1
		}
		r.count++
		if !r.captured {
			r.captured = true
			r.auth = req.Header.Get("Authorization")
			r.ct = req.Header.Get("Content-Type")
			r.body, _ = io.ReadAll(req.Body)
		}
		st := statuses[i]
		r.mu.Unlock()
		w.WriteHeader(st)
	}
}

func (r *recorder) firstRequest() (auth, ct string, body []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.auth, r.ct, r.body
}

// runLoop drives a Loop against a programmable server. The fake Sleep records
// each wait and returns context.Canceled on the maxSleeps-th call, so Run halts
// deterministically with exactly maxSleeps recorded waits.
func runLoop(t *testing.T, statuses []int, interval time.Duration, maxSleeps int) (sleeps []time.Duration, rec *recorder, runErr error) {
	t.Helper()
	rec = &recorder{}
	srv := httptest.NewServer(rec.handler(statuses))
	defer srv.Close()

	sleep := func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		if len(sleeps) >= maxSleeps {
			return context.Canceled
		}
		return nil
	}
	loop := &Loop{
		Client:   NewClient(srv.URL, "sekret"),
		Interval: interval,
		Next:     func() shared.ReportPayload { return fixedPayload() },
		Now:      func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		Sleep:    sleep,
	}
	runErr = loop.Run(context.Background())
	return sleeps, rec, runErr
}

func durationsEqual(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLoopBackoffSequence(t *testing.T) {
	sleeps, _, err := runLoop(t, []int{500, 500, 500, 500, 500, 500}, 10*time.Second, 5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err = %v, want context.Canceled", err)
	}
	want := []time.Duration{
		10 * time.Second, 20 * time.Second, 40 * time.Second,
		80 * time.Second, 120 * time.Second,
	}
	if !durationsEqual(sleeps, want) {
		t.Errorf("backoff sleeps = %v, want %v", sleeps, want)
	}
}

func TestLoopResetsAfterSuccess(t *testing.T) {
	sleeps, _, _ := runLoop(t, []int{500, 500, 500, 204, 204, 204}, 10*time.Second, 5)
	want := []time.Duration{
		10 * time.Second, 20 * time.Second, 40 * time.Second,
		10 * time.Second, 10 * time.Second,
	}
	if !durationsEqual(sleeps, want) {
		t.Errorf("sleeps = %v, want %v (reset to interval after success)", sleeps, want)
	}
}

func TestLoopPermanentJumpsToCap(t *testing.T) {
	sleeps, _, _ := runLoop(t, []int{401, 401, 401, 401}, 10*time.Second, 3)
	want := []time.Duration{120 * time.Second, 120 * time.Second, 120 * time.Second}
	if !durationsEqual(sleeps, want) {
		t.Errorf("permanent sleeps = %v, want all 2m (jump straight to cap)", sleeps)
	}
}

func TestLoopStampsReportedAtAndSendsContract(t *testing.T) {
	sleeps, rec, _ := runLoop(t, []int{204, 204}, 10*time.Second, 1)
	if len(sleeps) != 1 {
		t.Fatalf("sleeps = %v, want exactly one", sleeps)
	}
	auth, ct, body := rec.firstRequest()
	if auth != "Bearer sekret" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer sekret")
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	var p shared.ReportPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if p.DeviceType != "windows" {
		t.Errorf("DeviceType = %q, want windows", p.DeviceType)
	}
	want := time.Unix(1_700_000_000, 0).UTC()
	if !p.ReportedAt.Equal(want) {
		t.Errorf("ReportedAt = %v, want %v (loop must stamp UTC)", p.ReportedAt, want)
	}
}

func TestLoopCancelBeforeStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	loop := &Loop{
		Client:   NewClient(srv.URL, "sekret"),
		Interval: 10 * time.Second,
		Next:     func() shared.ReportPayload { return fixedPayload() },
		Sleep:    func(context.Context, time.Duration) error { return nil },
	}
	if err := loop.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Run err = %v, want context.Canceled (no send should happen)", err)
	}
}
