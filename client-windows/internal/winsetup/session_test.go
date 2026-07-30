//go:build windows

package winsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"cyberstalk.me/client-windows/internal/config"
	"cyberstalk.me/client-windows/internal/mapping"
	"cyberstalk.me/client-windows/internal/setup"
)

// session runs a real setup session — real port, real Win32 collectors, real
// file writes — and hands the test its address.
type session struct {
	t     *testing.T
	url   string
	token string
	path  string
	done  chan error
	stop  context.CancelFunc
}

func startSession(t *testing.T) *session {
	t.Helper()
	noBrowser := false
	path := filepath.Join(t.TempDir(), "config.yaml")
	ctx, cancel := context.WithCancel(context.Background())

	ready := make(chan string, 1)
	s := &session{t: t, path: path, done: make(chan error, 1), stop: cancel}
	go func() {
		s.done <- Run(ctx, Options{
			ConfigPath:      path,
			ObserveInterval: 10 * time.Millisecond,
			OpenBrowser:     &noBrowser,
			OnListen:        func(url string) { ready <- url },
			Index:           func(token string) ([]byte, error) { return []byte(token), nil },
		})
	}()

	select {
	case s.url = <-ready:
	case err := <-s.done:
		t.Fatalf("the session ended before it was listening: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the session never started listening")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.done:
		case <-time.After(10 * time.Second):
			t.Error("the session did not stop")
		}
	})

	// The page is what hands out the token, exactly as a browser would get it.
	resp, err := http.Get(s.url)
	if err != nil {
		t.Fatalf("fetch the index page: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the index page: %v", err)
	}
	s.token = string(body)
	if s.token == "" {
		t.Fatal("the index page carried no token")
	}
	return s
}

// request sends an authorized request unless token is overridden to "".
func (s *session) request(method, path string, body any, token string) *http.Response {
	s.t.Helper()
	var r io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			s.t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, strings.TrimSuffix(s.url, "/")+path, r)
	if err != nil {
		s.t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// AC7: the running port answers the UI and nothing else.
func TestLiveSessionGuardsItsPort(t *testing.T) {
	s := startSession(t)

	resp := s.request("GET", "/api/config", nil, "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("without a token: status = %d, want 401", resp.StatusCode)
	}

	resp = s.request("GET", "/api/config", nil, "not-the-token")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("with a wrong token: status = %d, want 401", resp.StatusCode)
	}

	// A page on another site, with the right token, still gets nowhere.
	req, err := http.NewRequest("GET", strings.TrimSuffix(s.url, "/")+"/api/config", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Origin", "https://evil.example")
	forged, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("forged-origin request: %v", err)
	}
	forged.Body.Close()
	if forged.StatusCode != http.StatusForbidden {
		t.Errorf("with a forged Origin: status = %d, want 403", forged.StatusCode)
	}

	resp = s.request("GET", "/api/config", nil, s.token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("with the session token: status = %d, want 200", resp.StatusCode)
	}
}

// R5.1: the port must not be reachable from anywhere but this machine.
func TestLiveSessionBindsLoopbackOnly(t *testing.T) {
	s := startSession(t)

	host, port, err := net.SplitHostPort(strings.TrimSuffix(strings.TrimPrefix(s.url, "http://"), "/"))
	if err != nil {
		t.Fatalf("parse %q: %v", s.url, err)
	}
	if host != "127.0.0.1" {
		t.Errorf("listening on %q, want 127.0.0.1", host)
	}

	// Binding the same port on a non-loopback address must still be possible,
	// which it would not be had the server bound 0.0.0.0.
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("cannot enumerate interfaces: %v", err)
	}
	tried := 0
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(ipnet.IP.String(), port))
		if err == nil {
			ln.Close()
			return // the port is free off-loopback, which is the whole point
		}
		if errors.Is(err, syscall.EADDRINUSE) {
			t.Errorf("port %s is already taken on %s: the server is not loopback-only", port, ipnet.IP)
			return
		}
		// Anything else means this particular address cannot be bound at all —
		// a disconnected adapter's link-local address, typically. That says
		// nothing about the server, so try the next one.
		t.Logf("skipping %s: %v", ipnet.IP, err)
		tried++
	}
	t.Skipf("no bindable non-loopback IPv4 address to test against (%d unusable)", tried)
}

// AC2: starting from an empty directory, the UI alone produces a configuration
// the agent can load.
func TestLiveSessionSavesAConfigFromScratch(t *testing.T) {
	s := startSession(t)

	// Nothing on disk yet, and the UI says so.
	resp := s.request("GET", "/api/config", nil, s.token)
	var state setup.State
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	resp.Body.Close()
	if state.Valid {
		t.Error("an empty configuration was reported as valid")
	}
	if state.Notice == "" {
		t.Error("a missing config file was not explained")
	}

	// Fill in what the form would.
	draft := state.Draft
	draft.ServerURL = "http://localhost:8080"
	draft.DeviceID = "win-desktop"
	draft.Token = "0123456789abcdef"
	draft.Interval = "15s"
	draft.DeviceName = "测试机"
	draft.Rules = append(draft.Rules, mapping.Rule{Process: "code.exe", App: "VS Code", Description: "在写代码"})

	resp = s.request("PUT", "/api/config", draft, s.token)
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode state after PUT: %v", err)
	}
	resp.Body.Close()
	if !state.Valid {
		t.Fatalf("the filled-in draft is not valid: %+v", state.Error)
	}

	resp = s.request("POST", "/api/save", nil, s.token)
	var result setup.SaveResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode save result: %v", err)
	}
	resp.Body.Close()
	if !result.Saved {
		t.Fatalf("save failed: %+v", result.Error)
	}

	// The real proof: the agent's own loader accepts it.
	cfg, err := config.Load(s.path)
	if err != nil {
		t.Fatalf("the saved configuration does not load: %v", err)
	}
	if cfg.DeviceID != "win-desktop" || cfg.Interval != 15*time.Second || len(cfg.Rules) != 1 {
		t.Errorf("loaded configuration = %+v", cfg)
	}

	// AC1: the file must not contain a raw window title. Whatever is in the
	// foreground while this test runs, none of it belongs in config.yaml.
	assertNoTitles(t, s.path)
}

// AC1: raw titles reach the browser and nothing else. The catalog holds them,
// so if anything is going to leak them it is a save.
func assertNoTitles(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// The observer has been sampling this machine's foreground window for the
	// length of the test; whatever it saw must not be in here.
	saved := string(data)
	catalog := setup.NewCatalog(setup.CatalogOptions{})
	fg := foreground()
	catalog.Observe(fg.Process, fg.Title())
	for _, app := range catalog.Snapshot().Apps {
		for _, sample := range app.Samples {
			if sample.Title != "" && strings.Contains(saved, sample.Title) {
				t.Errorf("the saved configuration contains a raw window title: %q", sample.Title)
			}
		}
	}
}
