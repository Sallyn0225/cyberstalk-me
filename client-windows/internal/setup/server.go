package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"cyberstalk.me/client-windows/internal/config"
	"cyberstalk.me/client-windows/internal/mapping"
	"cyberstalk.me/shared"
)

// DefaultIdleTimeout closes the setup session when nothing has talked to it for
// this long — a browser tab that was closed should not leave a port listening.
const DefaultIdleTimeout = 30 * time.Minute

// DefaultStreamInterval is how often the catalog stream checks for something
// new to send. It matches the observation rate: an app the user just switched
// to should show up in about a second.
const DefaultStreamInterval = time.Second

// Foreground is one reading of the foreground window.
//
// Title is a func for the same reason collect's is: the raw title is only read
// when something actually needs it.
type Foreground struct {
	Process     string
	Title       func() string
	IdleSeconds int
}

// Source produces the current foreground reading. cmd/agent supplies the Win32
// implementation; this package never touches Win32 itself, which is what keeps
// it testable on any platform.
type Source func() Foreground

// Options configures a Server.
type Options struct {
	// ConfigPath is the file a save writes to.
	ConfigPath string
	// Initial is the configuration the UI starts from. Required.
	Initial *config.Config
	// Notice explains why the UI is starting from defaults — an unreadable or
	// invalid config file. Empty when the file loaded cleanly.
	Notice string
	// Catalog receives observations; the UI streams it. Required.
	Catalog *Catalog
	// Source reads the current foreground window for the live preview.
	// Required.
	Source Source
	// IdleTimeout defaults to DefaultIdleTimeout. A negative value disables the
	// timeout.
	IdleTimeout time.Duration
	// StreamInterval defaults to DefaultStreamInterval.
	StreamInterval time.Duration
	// Index renders the UI's entry page, receiving the session token to embed.
	// Optional: without it, only the API is served.
	Index func(token string) ([]byte, error)
	// Assets is the UI's static bundle (JavaScript, CSS). Optional.
	Assets fs.FS
}

// Server is the local configuration UI's HTTP interface.
//
// It owns the draft configuration. The alternative — the browser owning it and
// posting the whole thing for every preview — would mean recompiling every
// regular expression on every keystroke, and would leave two copies of the
// rules to disagree with each other.
type Server struct {
	opts         Options
	token        string
	listener     net.Listener
	allowedHosts []string

	mu     sync.Mutex
	draft  *config.Config
	mapper *mapping.Mapper

	touchCh  chan struct{}
	quitCh   chan struct{}
	quitOnce sync.Once
}

// New builds a server around an initial configuration.
func New(opts Options) (*Server, error) {
	if opts.Initial == nil {
		return nil, errors.New("setup: Initial configuration is required")
	}
	if opts.Catalog == nil {
		return nil, errors.New("setup: Catalog is required")
	}
	if opts.Source == nil {
		return nil, errors.New("setup: Source is required")
	}
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = DefaultIdleTimeout
	}
	if opts.StreamInterval <= 0 {
		opts.StreamInterval = DefaultStreamInterval
	}

	token, err := newToken()
	if err != nil {
		return nil, err
	}
	// The initial configuration came either from a validated Load or from the
	// built-in defaults, so this compile is expected to succeed.
	mapper, err := mapping.New(opts.Initial.MapperOptions())
	if err != nil {
		return nil, fmt.Errorf("setup: compile initial rules: %w", err)
	}
	return &Server{
		opts:    opts,
		token:   token,
		draft:   opts.Initial,
		mapper:  mapper,
		touchCh: make(chan struct{}, 1),
		quitCh:  make(chan struct{}),
	}, nil
}

// Listen binds the loopback port the UI will be served on.
//
// The address is hardcoded to 127.0.0.1 and is not configurable: this server
// can read raw window titles, and the one thing it must never do is answer
// something off the machine. Port 0 lets the OS pick, so two agents never
// collide.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen on loopback: %w", err)
	}
	s.listener = ln
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		ln.Close()
		return fmt.Errorf("read listen port: %w", err)
	}
	s.allowedHosts = loopbackHosts(port)
	return nil
}

// URL is where the browser should be pointed. The token is deliberately absent:
// a URL ends up in history, in the window title, and in any Referer the page
// later sends. The token is handed to the page in its HTML instead.
func (s *Server) URL() string {
	if s.listener == nil {
		return ""
	}
	return "http://" + s.listener.Addr().String() + "/"
}

// Token is this session's bearer token, for the page to use.
func (s *Server) Token() string { return s.token }

// Serve runs until the UI asks to quit, ctx is cancelled, or nothing has
// happened for the idle timeout. It returns the reason it stopped, which is
// informational rather than a failure.
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	reason := s.wait(ctx, errCh)

	// A short grace period: the response to /api/quit still has to get out.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Debug("setup: shutdown", "err", err)
	}
	s.stop()
	return reason
}

// wait blocks until something ends the session and reports why.
func (s *Server) wait(ctx context.Context, errCh <-chan error) error {
	if s.opts.IdleTimeout < 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.quitCh:
			return nil
		case err := <-errCh:
			return err
		}
	}

	idle := time.NewTimer(s.opts.IdleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.quitCh:
			return nil
		case err := <-errCh:
			return err
		case <-s.touchCh:
			idle.Reset(s.opts.IdleTimeout)
		case <-idle.C:
			return fmt.Errorf("no activity for %s", s.opts.IdleTimeout)
		}
	}
}

// touch restarts the idle countdown. Non-blocking: a burst of requests only
// needs to reset the timer once.
func (s *Server) touch() {
	select {
	case s.touchCh <- struct{}{}:
	default:
	}
}

// stop ends the session. Safe to call more than once.
func (s *Server) stop() {
	s.quitOnce.Do(func() { close(s.quitCh) })
}

// Handler is the full route table.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/catalog", s.handleCatalog)
	api.HandleFunc("GET /api/config", s.handleGetConfig)
	api.HandleFunc("PUT /api/config", s.handlePutConfig)
	api.HandleFunc("GET /api/preview", s.handlePreview)
	api.HandleFunc("POST /api/regex/test", s.handleRegexTest)
	api.HandleFunc("POST /api/regex/suggest", s.handleRegexSuggest)
	api.HandleFunc("POST /api/save", s.handleSave)
	api.HandleFunc("POST /api/quit", s.handleQuit)

	mux := http.NewServeMux()
	mux.Handle("/api/", s.guard(api))
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

// handleIndex serves the UI: its static bundle, or the entry page with the
// session token embedded.
//
// The page is the only thing served without a token, because it is what hands
// the token out. It carries nothing else: a browser on another origin cannot
// read the response (no CORS headers are set), and a local program able to
// fetch it could already read the same window titles directly. The bundle
// itself is code, not data — there is nothing in it to protect.
//
// The Host check still applies, for the one case where "another origin" is not
// true: a DNS rebinding answer pointing an attacker's name at 127.0.0.1 makes
// this page same-origin for that name, and this page carries the token.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !s.hostAllowed(r.Host) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	s.touch()

	if r.URL.Path != "/" && s.opts.Assets != nil {
		if f, err := s.opts.Assets.Open(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
			f.Close()
			http.FileServerFS(s.opts.Assets).ServeHTTP(w, r)
			return
		}
	}

	if s.opts.Index == nil {
		http.NotFound(w, r)
		return
	}
	page, err := s.opts.Index(s.token)
	if err != nil {
		slog.Error("setup: render index", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page embeds a session token; caching it would outlive the session.
	w.Header().Set("Cache-Control", "no-store")
	w.Write(page)
}

// State is what the UI loads at startup.
type State struct {
	Draft      Draft  `json:"draft"`
	ConfigPath string `json:"config_path"`
	BackupPath string `json:"backup_path"`
	// Notice explains why the draft is the built-in defaults rather than the
	// file's contents.
	Notice string `json:"notice,omitempty"`
	// Defaults is what each key falls back to when left empty, so the form can
	// show it as a placeholder.
	Defaults Draft `json:"defaults"`
	// Valid reports whether the current draft would be accepted.
	Valid bool             `json:"valid"`
	Error *ValidationIssue `json:"error,omitempty"`
}

// ValidationIssue is a rejected configuration, in the shape a form needs.
type ValidationIssue struct {
	Message string `json:"message"`
	// Fields are YAML paths, e.g. "rules[1].title_patterns[0].match".
	Fields []string `json:"fields,omitempty"`
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	draft := DraftOf(s.draft)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, s.state(draft))
}

// handlePutConfig replaces the draft.
//
// A draft that does not validate is still accepted: it is what the user is
// currently typing, and refusing it would mean the form and the server disagree
// about what is on screen. The response says whether it would save.
func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var draft Draft
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	cfg, err := draft.Config()
	if err != nil {
		// Only a duration can fail this early; keep the previous draft and
		// report which field.
		writeJSON(w, http.StatusOK, s.stateWithIssue(draft, err))
		return
	}

	s.mu.Lock()
	s.draft = cfg
	// The mapper is only replaced when the draft compiles. A half-typed regular
	// expression must not take the live preview down with it.
	if mapper, mErr := mapping.New(cfg.MapperOptions()); mErr == nil {
		s.mapper = mapper
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, s.state(DraftOf(cfg)))
}

// state describes the current draft, validating it as it goes.
func (s *Server) state(draft Draft) State {
	var issue error
	if cfg, err := draft.Config(); err != nil {
		issue = err
	} else {
		issue = cfg.Validate(draftSource)
	}
	return s.stateWithIssue(draft, issue)
}

func (s *Server) stateWithIssue(draft Draft, issue error) State {
	st := State{
		Draft:      draft,
		ConfigPath: s.opts.ConfigPath,
		BackupPath: config.BackupPath(s.opts.ConfigPath),
		Notice:     s.opts.Notice,
		Defaults:   defaultDraft(),
		Valid:      issue == nil,
	}
	if issue != nil {
		st.Error = issueOf(issue)
	}
	return st
}

// issueOf renders an error for the form, keeping the field paths when the error
// carries them.
func issueOf(err error) *ValidationIssue {
	issue := &ValidationIssue{Message: err.Error()}
	var ve *config.ValidationError
	if errors.As(err, &ve) {
		issue.Message = ve.Message
		issue.Fields = ve.Fields
	}
	return issue
}

// defaultDraft is what an empty form falls back to, straight from the config
// package's defaults so the placeholder and the behaviour cannot drift.
func defaultDraft() Draft {
	cfg := &config.Config{}
	cfg.Normalize()
	return DraftOf(cfg)
}

// Preview is what the agent would report right now.
type Preview struct {
	Activity shared.Activity `json:"activity"`
	// Process is the current foreground executable, or "" when there is no
	// foreground window (the lock screen).
	Process string `json:"process"`
	// Configurable reports whether a rule can be written for this process.
	Configurable bool `json:"configurable"`
}

// handlePreview resolves the current foreground window through the draft.
//
// This is the same mapping.Resolve call the reporting loop and -dry-run make,
// against a mapper compiled from the same rules. There is deliberately no
// second implementation of the matching logic anywhere, in this package or in
// the browser: a preview that could disagree with the real thing would be worse
// than no preview.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	fg := s.opts.Source()

	s.mu.Lock()
	mapper := s.mapper
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, Preview{
		Activity:     mapper.Resolve(fg.Process, fg.Title, fg.IdleSeconds),
		Process:      fg.Process,
		Configurable: fg.Process != "" && isConfigurable(fg.Process),
	})
}

// RegexTest asks how a pattern behaves against what has been seen.
type RegexTest struct {
	Pattern string `json:"pattern"`
	// Process selects whose title samples to test against. Empty means every
	// sample in the catalog.
	Process string `json:"process"`
}

// RegexTestResult reports the outcome, with the titles it was tested against so
// the UI can highlight them without guessing at ordering.
type RegexTestResult struct {
	PatternTest
	Titles []string `json:"titles"`
}

func (s *Server) handleRegexTest(w http.ResponseWriter, r *http.Request) {
	var req RegexTest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	titles := s.opts.Catalog.Titles(req.Process)
	if req.Process == "" {
		titles = nil
		for _, app := range s.opts.Catalog.Snapshot().Apps {
			for _, sample := range app.Samples {
				titles = append(titles, sample.Title)
			}
		}
	}
	writeJSON(w, http.StatusOK, RegexTestResult{
		PatternTest: TestPattern(req.Pattern, titles),
		Titles:      titles,
	})
}

// handleRegexSuggest turns a title sample into a starting pattern.
//
// The escaping happens here rather than in the browser because Go's regexp
// escaping and JavaScript's are not the same set of characters. A pattern
// escaped by the browser could compile differently, or not at all, in the agent
// that has to run it.
func (s *Server) handleRegexSuggest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pattern": SuggestPattern(req.Title)})
}

// SaveResult is the outcome of writing the configuration.
type SaveResult struct {
	Saved      bool   `json:"saved"`
	ConfigPath string `json:"config_path"`
	// BackupPath is where the previous file went, empty when there was none.
	BackupPath string           `json:"backup_path,omitempty"`
	Error      *ValidationIssue `json:"error,omitempty"`
}

// handleSave writes the draft to disk.
//
// Validation is config.Save's, which is Load's — a configuration this accepts
// is one the agent will start with, and a configuration it rejects is rejected
// with the same message the agent would print.
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cfg := s.draft
	s.mu.Unlock()

	hadFile := fileExists(s.opts.ConfigPath)
	if err := config.Save(s.opts.ConfigPath, cfg); err != nil {
		var ve *config.ValidationError
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, SaveResult{
				ConfigPath: s.opts.ConfigPath,
				Error:      issueOf(err),
			})
			return
		}
		// A write failure is not the user's mistake; it needs a real log line.
		slog.Error("setup: save config", "path", s.opts.ConfigPath, "err", err)
		writeJSON(w, http.StatusInternalServerError, SaveResult{
			ConfigPath: s.opts.ConfigPath,
			Error:      &ValidationIssue{Message: err.Error()},
		})
		return
	}

	result := SaveResult{Saved: true, ConfigPath: s.opts.ConfigPath}
	if hadFile {
		result.BackupPath = config.BackupPath(s.opts.ConfigPath)
	}
	slog.Info("setup: configuration saved", "path", s.opts.ConfigPath, "rules", len(cfg.Rules))
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"stopping": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.stop()
}

// handleCatalog streams the catalog as server-sent events.
//
// PRIVACY: this is the one route that carries raw window titles. It is reachable
// only over loopback, only with this session's token, and only from this
// session's own page.
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	send := func(snap CatalogSnapshot) bool {
		if _, err := w.Write([]byte("data: ")); err != nil {
			return false
		}
		if err := enc.Encode(snap); err != nil {
			return false
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	snap := s.opts.Catalog.Snapshot()
	if !send(snap) {
		return
	}
	seen := snap.Observations

	ticker := time.NewTicker(s.opts.StreamInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.quitCh:
			return
		case <-ticker.C:
			snap := s.opts.Catalog.Snapshot()
			if snap.Observations == seen {
				continue
			}
			seen = snap.Observations
			if !send(snap) {
				return
			}
			// An open stream means the page is still there; it should not time
			// out while the user is looking at it.
			s.touch()
		}
	}
}

// fileExists reports whether a configuration is already on disk, which decides
// whether a save produces a backup.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Debug("setup: write response", "err", err)
	}
}

// writeError matches the server's error shape: a single "error" key.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
