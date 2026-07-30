package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cyberstalk.me/client-windows/internal/config"
	"cyberstalk.me/client-windows/internal/mapping"
	"cyberstalk.me/shared"
)

// canaryTitle stands in for a raw window title. It must never appear anywhere
// except a deliberately authorized response body.
const canaryTitle = "SETUP-CANARY — 机密项目"

func testConfig() *config.Config {
	cfg := &config.Config{
		ServerURL: "http://localhost:8080",
		DeviceID:  "win-desktop",
		Token:     "test-token",
		Rules: []mapping.Rule{
			{Process: "code.exe", App: "VS Code", Description: "在写代码"},
			{Process: "chrome.exe", App: "Chrome", Description: "在上网", TitlePatterns: []mapping.TitlePattern{
				{Match: "(?i)youtube", Description: "在看视频"},
			}},
		},
	}
	cfg.Normalize()
	return cfg
}

// harness is a server plus the knobs a test needs to drive it.
type harness struct {
	*Server
	catalog *Catalog
	fg      Foreground
}

func newHarness(t *testing.T, mutate func(*Options)) *harness {
	t.Helper()
	h := &harness{
		catalog: NewCatalog(CatalogOptions{}),
		fg:      Foreground{Process: "code.exe", Title: func() string { return canaryTitle }},
	}
	opts := Options{
		ConfigPath:  t.TempDir() + "/config.yaml",
		Initial:     testConfig(),
		Catalog:     h.catalog,
		Source:      func() Foreground { return h.fg },
		IdleTimeout: -1, // no timeout unless a test asks for one
	}
	if mutate != nil {
		mutate(&opts)
	}
	srv, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.allowedHosts = loopbackHosts("1234")
	h.Server = srv
	return h
}

// do sends an authorized request, the way the real page would.
func (h *harness) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		r = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, r)
	req.Host = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+h.Token())
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %T from %s: %v", out, rec.Body.String(), err)
	}
	return out
}

// AC7: without the session token, nothing is reachable.
func TestGuardRequiresTheSessionToken(t *testing.T) {
	h := newHarness(t, nil)
	routes := []struct{ method, path string }{
		{"GET", "/api/config"},
		{"PUT", "/api/config"},
		{"GET", "/api/preview"},
		{"GET", "/api/catalog"},
		{"POST", "/api/regex/test"},
		{"POST", "/api/regex/suggest"},
		{"POST", "/api/save"},
		{"POST", "/api/quit"},
	}
	headers := []struct {
		name  string
		value string
	}{
		{"no header", ""},
		{"empty bearer", "Bearer "},
		{"wrong token", "Bearer not-the-token"},
		{"right token, wrong scheme", "Token %s"},
		{"no scheme", "%s"},
	}
	for _, route := range routes {
		for _, hdr := range headers {
			t.Run(route.method+" "+route.path+" "+hdr.name, func(t *testing.T) {
				req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
				req.Host = "127.0.0.1:1234"
				if hdr.value != "" {
					req.Header.Set("Authorization", strings.ReplaceAll(hdr.value, "%s", h.Token()))
				}
				rec := httptest.NewRecorder()
				h.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusUnauthorized {
					t.Errorf("status = %d, want 401", rec.Code)
				}
				if strings.Contains(rec.Body.String(), h.Token()) {
					t.Error("the rejection echoed the session token")
				}
			})
		}
	}
}

// AC7: a page on another origin must not be able to talk to the API, and
// neither must a name that resolved to 127.0.0.1.
func TestGuardRejectsForeignOriginsAndHosts(t *testing.T) {
	h := newHarness(t, nil)
	tests := []struct {
		name   string
		host   string
		origin string
	}{
		{"foreign origin", "127.0.0.1:1234", "https://evil.example"},
		{"foreign origin over http", "127.0.0.1:1234", "http://evil.example"},
		{"loopback origin on another port", "127.0.0.1:1234", "http://127.0.0.1:9999"},
		{"rebound host name", "evil.example", ""},
		{"rebound host name with a good origin", "evil.example", "http://127.0.0.1:1234"},
		{"loopback host on another port", "127.0.0.1:9999", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/config", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set("Authorization", "Bearer "+h.Token())
			rec := httptest.NewRecorder()
			h.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
			if strings.Contains(rec.Body.String(), canaryTitle) {
				t.Error("a rejected request still saw a window title")
			}
		})
	}
}

func TestGuardAcceptsItsOwnOrigins(t *testing.T) {
	h := newHarness(t, nil)
	for _, origin := range []string{"", "http://127.0.0.1:1234", "http://localhost:1234"} {
		req := httptest.NewRequest("GET", "/api/config", nil)
		req.Host = "localhost:1234"
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.Header.Set("Authorization", "Bearer "+h.Token())
		rec := httptest.NewRecorder()
		h.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("origin %q: status = %d, want 200", origin, rec.Code)
		}
	}
}

// Two sessions must not accept each other's tokens.
func TestTokensAreUniquePerSession(t *testing.T) {
	a := newHarness(t, nil)
	b := newHarness(t, nil)
	if a.Token() == b.Token() {
		t.Fatal("two sessions produced the same token")
	}
	if len(a.Token()) < 32 {
		t.Errorf("token is %d characters, want a long random one", len(a.Token()))
	}

	req := httptest.NewRequest("GET", "/api/config", nil)
	req.Host = "127.0.0.1:1234"
	req.Header.Set("Authorization", "Bearer "+b.Token())
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for another session's token", rec.Code)
	}
}

func TestGetConfigReturnsTheDraftAndDefaults(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.Notice = "config.yaml 读不出来，先用默认值" })

	rec := h.do(t, "GET", "/api/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	state := decode[State](t, rec)
	if !state.Valid || state.Error != nil {
		t.Errorf("Valid = %v, Error = %+v, want a valid draft", state.Valid, state.Error)
	}
	if state.Draft.DeviceID != "win-desktop" || state.Draft.Token != "test-token" {
		t.Errorf("draft identity = %+v", state.Draft)
	}
	if state.Draft.Interval != "10s" {
		t.Errorf("Interval = %q, want the file spelling 10s", state.Draft.Interval)
	}
	if state.Notice == "" {
		t.Error("Notice was dropped")
	}
	if state.Defaults.DefaultApp != config.DefaultApp || state.Defaults.IdleThreshold != "5m" {
		t.Errorf("Defaults = %+v, want the config package's defaults", state.Defaults)
	}
	if state.BackupPath != state.ConfigPath+".bak" {
		t.Errorf("BackupPath = %q, want %q", state.BackupPath, state.ConfigPath+".bak")
	}
}

// R3.6: a problem shows up while editing, not at save time.
func TestPutConfigReportsProblemsWithoutLosingTheEdit(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*Draft)
		wantFields []string
		wantMsg    string
	}{
		{
			name:       "duplicate process",
			mutate:     func(d *Draft) { d.Rules = append(d.Rules, mapping.Rule{Process: "CODE.EXE", App: "又一个"}) },
			wantFields: []string{"rules[2].process"},
			wantMsg:    "duplicate process",
		},
		{
			name:       "empty app",
			mutate:     func(d *Draft) { d.Rules[0].App = "" },
			wantFields: []string{"rules[0].app"},
			wantMsg:    "app must not be empty",
		},
		{
			name: "invalid regexp",
			mutate: func(d *Draft) {
				d.Rules[1].TitlePatterns[0].Match = "(["
			},
			wantFields: []string{"rules[1].title_patterns[0].match"},
			wantMsg:    "compile",
		},
		{
			name:       "expose_title without a rule",
			mutate:     func(d *Draft) { d.ExposeTitle = []string{"ghost.exe"} },
			wantFields: []string{"expose_title[0]"},
			wantMsg:    "has no rule",
		},
		{
			name:       "missing token",
			mutate:     func(d *Draft) { d.Token = "" },
			wantFields: []string{"token"},
			wantMsg:    "must not be empty",
		},
		{
			name:       "unparseable interval",
			mutate:     func(d *Draft) { d.Interval = "soon" },
			wantFields: []string{"interval"},
			wantMsg:    "invalid duration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, nil)
			draft := DraftOf(testConfig())
			tt.mutate(&draft)

			rec := h.do(t, "PUT", "/api/config", draft)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			state := decode[State](t, rec)
			if state.Valid {
				t.Fatalf("Valid = true for %s", tt.name)
			}
			if state.Error == nil {
				t.Fatal("Error is nil")
			}
			if !strings.Contains(state.Error.Message, tt.wantMsg) {
				t.Errorf("Message = %q, want it to contain %q", state.Error.Message, tt.wantMsg)
			}
			if strings.Join(state.Error.Fields, ",") != strings.Join(tt.wantFields, ",") {
				t.Errorf("Fields = %v, want %v", state.Error.Fields, tt.wantFields)
			}
		})
	}
}

// AC6: what the UI refuses and what the agent refuses must be the same words.
func TestSaveRejectionMatchesStartupRejection(t *testing.T) {
	h := newHarness(t, nil)
	draft := DraftOf(testConfig())
	draft.Rules = append(draft.Rules, mapping.Rule{Process: "code.exe", App: "重复"})
	h.do(t, "PUT", "/api/config", draft)

	rec := h.do(t, "POST", "/api/save", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	result := decode[SaveResult](t, rec)
	if result.Saved {
		t.Fatal("Saved = true for an invalid configuration")
	}

	// The same configuration, validated the way startup validates it.
	cfg, err := draft.Config()
	if err != nil {
		t.Fatalf("draft.Config: %v", err)
	}
	startupErr := cfg.Validate(draftSource)
	if startupErr == nil {
		t.Fatal("startup validation accepted what the UI rejected")
	}
	var ve *config.ValidationError
	if !errors.As(startupErr, &ve) {
		t.Fatalf("startup error is %T", startupErr)
	}
	if result.Error.Message != ve.Message {
		t.Errorf("UI message  = %q\nstartup     = %q", result.Error.Message, ve.Message)
	}
	if _, err := config.Load(h.opts.ConfigPath); err == nil {
		t.Error("an invalid configuration was written to disk")
	}
}

func TestSaveWritesAndBacksUp(t *testing.T) {
	h := newHarness(t, nil)

	rec := h.do(t, "POST", "/api/save", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	first := decode[SaveResult](t, rec)
	if !first.Saved {
		t.Fatalf("Saved = false: %+v", first.Error)
	}
	if first.BackupPath != "" {
		t.Errorf("BackupPath = %q on a first save, want empty", first.BackupPath)
	}
	if _, err := config.Load(h.opts.ConfigPath); err != nil {
		t.Fatalf("the saved file does not load: %v", err)
	}

	// A second save has something to back up.
	second := decode[SaveResult](t, h.do(t, "POST", "/api/save", nil))
	if !second.Saved || second.BackupPath == "" {
		t.Errorf("second save = %+v, want a backup path", second)
	}
}

// AC4: the preview is the same resolution the reporting loop performs.
func TestPreviewMatchesMappingResolve(t *testing.T) {
	tests := []struct {
		name            string
		fg              Foreground
		wantApp         string
		wantDescription string
		wantLocked      bool
	}{
		{
			name:            "rule hit",
			fg:              Foreground{Process: "code.exe", Title: func() string { return canaryTitle }},
			wantApp:         "VS Code",
			wantDescription: "在写代码",
		},
		{
			name:            "title pattern hit",
			fg:              Foreground{Process: "chrome.exe", Title: func() string { return "YouTube - " + canaryTitle }},
			wantApp:         "Chrome",
			wantDescription: "在看视频",
		},
		{
			name:            "title pattern miss falls back to the rule",
			fg:              Foreground{Process: "chrome.exe", Title: func() string { return canaryTitle }},
			wantApp:         "Chrome",
			wantDescription: "在上网",
		},
		{
			name:            "no rule",
			fg:              Foreground{Process: "mystery.exe", Title: func() string { return canaryTitle }},
			wantApp:         config.DefaultApp,
			wantDescription: config.DefaultDescription,
		},
		{
			name:            "locked",
			fg:              Foreground{Process: ""},
			wantApp:         config.DefaultLockedApp,
			wantDescription: config.DefaultLockedDesc,
			wantLocked:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, nil)
			h.fg = tt.fg

			preview := decode[Preview](t, h.do(t, "GET", "/api/preview", nil))
			if preview.Activity.App != tt.wantApp {
				t.Errorf("App = %q, want %q", preview.Activity.App, tt.wantApp)
			}
			if preview.Activity.Description != tt.wantDescription {
				t.Errorf("Description = %q, want %q", preview.Activity.Description, tt.wantDescription)
			}
			if preview.Activity.Locked != tt.wantLocked {
				t.Errorf("Locked = %v, want %v", preview.Activity.Locked, tt.wantLocked)
			}

			// The same inputs through the mapping package directly must agree:
			// there is only one implementation, and this is the proof.
			mapper, err := mapping.New(testConfig().MapperOptions())
			if err != nil {
				t.Fatalf("mapping.New: %v", err)
			}
			want := mapper.Resolve(tt.fg.Process, tt.fg.Title, tt.fg.IdleSeconds)
			if preview.Activity != want {
				t.Errorf("preview = %+v, mapping.Resolve = %+v", preview.Activity, want)
			}
		})
	}
}

// AC1: an unmapped process must not leak its title into the preview, and the
// draft's own rules decide that — not anything the UI does.
func TestPreviewNeverLeaksAnUnmappedTitle(t *testing.T) {
	h := newHarness(t, nil)
	h.fg = Foreground{Process: "secret-tool.exe", Title: func() string { return canaryTitle }}

	rec := h.do(t, "GET", "/api/preview", nil)
	if strings.Contains(rec.Body.String(), canaryTitle) {
		t.Errorf("the preview leaked a raw title for an unmapped process: %s", rec.Body)
	}
}

// The live preview must survive a half-typed regular expression.
func TestPreviewKeepsWorkingWhileTheDraftIsBroken(t *testing.T) {
	h := newHarness(t, nil)
	h.fg = Foreground{Process: "code.exe", Title: func() string { return canaryTitle }}

	draft := DraftOf(testConfig())
	draft.Rules[1].TitlePatterns[0].Match = "(?i)(you"
	state := decode[State](t, h.do(t, "PUT", "/api/config", draft))
	if state.Valid {
		t.Fatal("an uncompilable pattern was accepted as valid")
	}

	preview := decode[Preview](t, h.do(t, "GET", "/api/preview", nil))
	if preview.Activity.App != "VS Code" {
		t.Errorf("preview broke while the draft was invalid: %+v", preview.Activity)
	}
}

func TestRegexTestAgainstCatalogSamples(t *testing.T) {
	h := newHarness(t, nil)
	h.catalog.Observe("chrome.exe", "YouTube - 音乐")
	h.catalog.Observe("chrome.exe", "GitHub - anthropics")
	h.catalog.Observe("code.exe", "main.go")

	result := decode[RegexTestResult](t, h.do(t, "POST", "/api/regex/test",
		RegexTest{Pattern: "(?i)youtube", Process: "chrome.exe"}))
	if !result.Valid {
		t.Fatalf("Valid = false: %s", result.Error)
	}
	if result.MatchCount != 1 {
		t.Errorf("MatchCount = %d, want 1", result.MatchCount)
	}
	if len(result.Titles) != 2 {
		t.Fatalf("Titles = %v, want the two chrome samples", result.Titles)
	}
	// Matched is parallel to Titles so the UI can highlight without guessing.
	for i, title := range result.Titles {
		want := strings.Contains(strings.ToLower(title), "youtube")
		if result.Matched[i] != want {
			t.Errorf("Matched[%d] = %v for %q", i, result.Matched[i], title)
		}
	}

	// An unparseable pattern is a normal state while typing, not a 400.
	bad := decode[RegexTestResult](t, h.do(t, "POST", "/api/regex/test",
		RegexTest{Pattern: "([", Process: "chrome.exe"}))
	if bad.Valid || bad.Error == "" {
		t.Errorf("an invalid pattern = %+v, want Valid false with a message", bad)
	}
}

// The suggestion has to be escaped by Go, not by the browser: a pattern the UI
// offers must be one the agent will accept and match identically.
func TestRegexSuggestEscapesWithGoRules(t *testing.T) {
	h := newHarness(t, nil)
	title := `[Debug] main.go (1/2) - 100% + a|b`

	result := decode[map[string]string](t, h.do(t, "POST", "/api/regex/suggest",
		map[string]string{"title": title}))
	pattern := result["pattern"]
	if pattern != SuggestPattern(title) {
		t.Errorf("pattern = %q, want %q", pattern, SuggestPattern(title))
	}

	// The round trip that matters: the suggestion matches the sample it came
	// from, through the same tester the UI highlights with.
	got := TestPattern(pattern, []string{title, "something else"})
	if !got.Valid || got.MatchCount != 1 || !got.Matched[0] {
		t.Errorf("suggested pattern does not match its own sample: %+v", got)
	}
}

func TestCatalogStreamSendsTheCurrentStateThenUpdates(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.StreamInterval = 5 * time.Millisecond })
	h.catalog.Observe("code.exe", canaryTitle)

	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	h.allowedHosts = []string{strings.TrimPrefix(srv.URL, "http://")}

	req, err := http.NewRequest("GET", srv.URL+"/api/catalog", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.Token())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}

	first := readEvent(t, resp.Body)
	if len(first.Apps) != 1 || first.Apps[0].Process != "code.exe" {
		t.Fatalf("first event = %+v, want the app already in the catalog", first.Apps)
	}
	if len(first.Apps[0].Samples) != 1 || first.Apps[0].Samples[0].Title != canaryTitle {
		t.Errorf("first event samples = %+v", first.Apps[0].Samples)
	}

	// R2.3: switching to a new app shows up without the page asking again.
	h.catalog.Observe("chrome.exe", "YouTube")
	second := readEvent(t, resp.Body)
	if len(second.Apps) != 2 {
		t.Errorf("second event = %+v, want both apps", second.Apps)
	}
}

// readEvent reads one "data: {...}" frame.
func readEvent(t *testing.T, r io.Reader) CatalogSnapshot {
	t.Helper()
	buf := make([]byte, 1)
	var line []byte
	for {
		n, err := r.Read(buf)
		if err != nil {
			t.Fatalf("read event: %v (so far: %s)", err, line)
		}
		if n == 0 {
			continue
		}
		if buf[0] == '\n' {
			if len(bytes.TrimSpace(line)) == 0 {
				line = nil
				continue
			}
			payload, ok := bytes.CutPrefix(line, []byte("data: "))
			if !ok {
				t.Fatalf("unexpected stream line %q", line)
			}
			var snap CatalogSnapshot
			if err := json.Unmarshal(payload, &snap); err != nil {
				t.Fatalf("decode event %s: %v", payload, err)
			}
			return snap
		}
		line = append(line, buf[0])
	}
}

// R1.5 / AC7: the UI can end the session, and Serve returns so the process can
// exit and stop listening.
func TestQuitEndsTheSession(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := strings.TrimPrefix(h.URL(), "http://")

	done := make(chan error, 1)
	go func() { done <- h.Serve(context.Background()) }()

	req, err := http.NewRequest("POST", h.URL()+"api/quit", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("quit: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v, want nil after a quit", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after /api/quit")
	}

	// AC7: nothing is listening once the session is over.
	if _, err := http.Get("http://" + strings.TrimSuffix(addr, "/")); err == nil {
		t.Error("the port still answers after the session ended")
	}
}

// R1.5: a browser tab that was closed must not leave the agent running forever.
func TestIdleTimeoutEndsTheSession(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.IdleTimeout = 80 * time.Millisecond })
	if err := h.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- h.Serve(context.Background()) }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "no activity") {
			t.Errorf("Serve returned %v, want an idle-timeout reason", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the idle timeout never fired")
	}
}

// Requests keep the session alive.
func TestRequestsPostponeTheIdleTimeout(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.IdleTimeout = 150 * time.Millisecond })
	if err := h.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	h.allowedHosts = loopbackHosts(portOf(t, h.URL()))

	done := make(chan error, 1)
	go func() { done <- h.Serve(context.Background()) }()

	client := &http.Client{}
	for i := 0; i < 4; i++ {
		time.Sleep(50 * time.Millisecond)
		req, err := http.NewRequest("GET", h.URL()+"api/config", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+h.Token())
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d after %dms: %v", i, (i+1)*50, err)
		}
		resp.Body.Close()
		select {
		case err := <-done:
			t.Fatalf("session ended early while requests were arriving: %v", err)
		default:
		}
	}
	h.stop()
	<-done
}

func portOf(t *testing.T, rawURL string) string {
	t.Helper()
	trimmed := strings.TrimSuffix(strings.TrimPrefix(rawURL, "http://"), "/")
	i := strings.LastIndex(trimmed, ":")
	if i < 0 {
		t.Fatalf("no port in %q", rawURL)
	}
	return trimmed[i+1:]
}

// The index page is what hands the token to the UI, so it must be served
// without one — and must carry nothing else.
func TestIndexEmbedsTheToken(t *testing.T) {
	h := newHarness(t, func(o *Options) {
		o.Index = func(token string) ([]byte, error) {
			return []byte("<html>" + token + "</html>"), nil
		}
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without a token", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), h.Token()) {
		t.Error("the page does not carry the session token")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store for a page holding a token", got)
	}
}

// The page hands out the session token, so a name that merely resolved to
// 127.0.0.1 must not be able to collect it.
func TestIndexRejectsAReboundHost(t *testing.T) {
	h := newHarness(t, func(o *Options) {
		o.Index = func(token string) ([]byte, error) {
			return []byte("<html>" + token + "</html>"), nil
		}
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if strings.Contains(rec.Body.String(), h.Token()) {
		t.Error("the rejection handed out the session token")
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{"no initial config", Options{Catalog: NewCatalog(CatalogOptions{}), Source: func() Foreground { return Foreground{} }}},
		{"no catalog", Options{Initial: testConfig(), Source: func() Foreground { return Foreground{} }}},
		{"no source", Options{Initial: testConfig(), Catalog: NewCatalog(CatalogOptions{})}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opts); err == nil {
				t.Error("New succeeded without a required dependency")
			}
		})
	}
}

// Activity is the contract type; the preview must not invent its own shape.
func TestPreviewUsesTheContractType(t *testing.T) {
	var _ shared.Activity = Preview{}.Activity
}
