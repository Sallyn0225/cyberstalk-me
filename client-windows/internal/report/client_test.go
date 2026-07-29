package report

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cyberstalk.me/shared"
)

// testPayload is a realistic, already-sanitized payload the client would send.
func testPayload() shared.ReportPayload {
	return shared.ReportPayload{
		DeviceID:   "win-desktop",
		DeviceName: "我的台式机",
		DeviceType: "windows",
		Activity:   shared.Activity{App: "VS Code", Description: "在写代码", IdleSeconds: 42},
	}
}

func TestSendAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sekret" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer sekret")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var p shared.ReportPayload
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if p.DeviceType != "windows" {
			t.Errorf("DeviceType = %q, want %q", p.DeviceType, "windows")
		}
		if p.DeviceID != "win-desktop" {
			t.Errorf("DeviceID = %q, want %q", p.DeviceID, "win-desktop")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "sekret").Send(context.Background(), testPayload()); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestSendStatusMapping(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantOK   bool // err == nil
		wantPerm bool // errors.Is(err, ErrPermanent)
	}{
		{"204 accepted", http.StatusNoContent, true, false},
		{"400 bad request is permanent", http.StatusBadRequest, false, true},
		{"401 unauthorized is permanent", http.StatusUnauthorized, false, true},
		{"500 server error is retryable", http.StatusInternalServerError, false, false},
		{"503 service unavailable is retryable", http.StatusServiceUnavailable, false, false},
		{"302 redirect is permanent (unexpected)", http.StatusFound, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			err := NewClient(srv.URL, "sekret").Send(context.Background(), testPayload())
			if tt.wantOK {
				if err != nil {
					t.Fatalf("Send: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Send succeeded, want error")
			}
			if got := errors.Is(err, ErrPermanent); got != tt.wantPerm {
				t.Errorf("errors.Is(err, ErrPermanent) = %v, want %v (err=%v)", got, tt.wantPerm, err)
			}
			// No status-code-only error message may leak the token.
			if strings.Contains(err.Error(), "sekret") {
				t.Errorf("error leaks the token: %v", err)
			}
		})
	}
}

func TestSendNetworkErrorIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // now unreachable

	err := NewClient(srv.URL, "sekret").Send(context.Background(), testPayload())
	if err == nil {
		t.Fatal("Send succeeded against a closed server, want error")
	}
	if errors.Is(err, ErrPermanent) {
		t.Errorf("network error is permanent, want retryable: %v", err)
	}
	if strings.Contains(err.Error(), "sekret") {
		t.Errorf("error leaks the token: %v", err)
	}
}
