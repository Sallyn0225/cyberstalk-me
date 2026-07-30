// Command server is the cyberstalk-me backend. It serves the JSON API
// (report/snapshot/stream), the SSE stream, and the embedded frontend.
//
// It also has an admin subcommand:
//
//	cyberstalk-server register-device <id> <name> <type> [--server-url URL]
//
// which creates a device, prints a one-time token, and prints a client
// config snippet. Wiring (DB, hub, tracker, router) happens here, never in
// package init().
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embed the IANA timezone database. DISPLAY_TIMEZONE is resolved with
	// time.LoadLocation, and the runtime image is alpine, which ships no
	// tzdata — without this the server fails to start in the container while
	// working fine everywhere else. The Dockerfile's runtime stage must stay
	// RUN-free (adding `apk add tzdata` would drag QEMU into the multi-arch
	// build), so the database goes in the binary instead. Costs ~450 KB.
	_ "time/tzdata"

	"cyberstalk.me/server/internal/api"
	"cyberstalk.me/server/internal/config"
	"cyberstalk.me/server/internal/hub"
	"cyberstalk.me/server/internal/state"
	"cyberstalk.me/server/internal/store"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// webStatic is the embedded static frontend. For now this is a single
// placeholder page; the web subtask (07-28-web) will swap in the Vite build
// output (web/dist) by pointing this embed at the built directory.
//
//go:embed all:web
var webStatic embed.FS

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 1 && args[1] == "register-device" {
		return runRegister(args[2:])
	}
	return runServer()
}

// runServer loads config, opens the DB, wires everything, and serves until
// SIGINT/SIGTERM. On shutdown it waits for in-flight requests and closes SSE
// connections cleanly.
func runServer() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := openDB(ctx, cfg.SQLitePath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	st, err := store.New(ctx, db)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}

	h := hub.New()
	tracker := state.New(st, h, cfg.OfflineThreshold, cfg.ScanInterval, time.Now)
	handlers := api.New(st, h, tracker, time.Now, cfg.UsageMaxGap, cfg.Location)

	// Serve the embedded web/ directory at the root. fs.Sub strips the
	// "web" prefix so files appear at "/".
	webFS, err := fs.Sub(webStatic, "web")
	if err != nil {
		return fmt.Errorf("embed web subdir: %w", err)
	}
	router := api.Router(handlers, webFS)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go tracker.Run(ctx)
	go pruneUsage(ctx, st, cfg.UsageRetentionDays, cfg.UsagePruneInterval)
	go func() {
		slog.Info("server starting", "addr", cfg.Addr, "offline_threshold", cfg.OfflineThreshold,
			"scan_interval", cfg.ScanInterval, "display_timezone", cfg.DisplayTimezone,
			"usage_retention_days", cfg.UsageRetentionDays, "usage_max_gap", cfg.UsageMaxGap)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen and serve", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	slog.Info("server stopped")
	return nil
}

// pruneUsage enforces the usage retention window until ctx is cancelled. It
// runs once immediately so a server that restarts more often than the prune
// interval still prunes, then on every tick.
//
// A failed prune is logged and retried on the next tick: retention is
// housekeeping, and being unable to delete old rows is not a reason to take
// the server down.
func pruneUsage(ctx context.Context, st *store.Store, retentionDays int, interval time.Duration) {
	prune := func() {
		before := time.Now().UTC().AddDate(0, 0, -retentionDays)
		n, err := st.PruneUsage(ctx, before)
		if err != nil {
			slog.Error("prune usage", "err", err)
			return
		}
		if n > 0 {
			slog.Info("pruned usage buckets", "rows", n, "before", before.Format(time.RFC3339))
		}
	}

	prune()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}

// openDB opens the SQLite database. ":memory:" is supported for tests; the
// modernc driver uses the "sqlite" scheme via the file DSN.
func openDB(ctx context.Context, path string) (*sql.DB, error) {
	dsn := path
	if path == ":memory:" {
		// Shared cache keeps the in-memory DB consistent across the
		// single connection pool used by tests.
		dsn = "file::memory:?cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	// Validate the connection now so a bad path fails at startup.
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	// Single writer for SQLite. A pool of 1 avoids "database is locked"
	// under contention in tests.
	db.SetMaxOpenConns(1)
	return db, nil
}

// runRegister implements `register-device <id> <name> <type>`. It generates a
// random token, stores its SHA-256 hash, and prints the plaintext token plus
// a client config snippet once. The plaintext token is never stored.
//
// Flags may appear before or after the positional arguments for ergonomics.
func runRegister(args []string) error {
	serverURL := "http://localhost:8080"
	sqlitePath := os.Getenv("SQLITE_PATH")
	if sqlitePath == "" {
		sqlitePath = "cyberstalk.db"
	}

	// Separate flags from positional args so users can write either
	// `register-device <id> <name> <type> --flag v` or
	// `register-device --flag v <id> <name> <type>`.
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--server-url":
			if i+1 >= len(args) {
				return errors.New("--server-url requires a value")
			}
			serverURL = args[i+1]
			i++
		case "--sqlite-path":
			if i+1 >= len(args) {
				return errors.New("--sqlite-path requires a value")
			}
			sqlitePath = args[i+1]
			i++
		case "-h", "--help":
			return errors.New("usage: register-device <id> <name> <type> [--server-url URL] [--sqlite-path PATH]")
		default:
			positional = append(positional, a)
		}
	}

	if len(positional) != 3 {
		return errors.New("usage: register-device <id> <name> <type> [--server-url URL] [--sqlite-path PATH]")
	}
	id, name, deviceType := positional[0], positional[1], positional[2]
	if deviceType != "windows" && deviceType != "android" {
		return fmt.Errorf("type must be windows or android, got %q", deviceType)
	}

	ctx := context.Background()
	db, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	st, err := store.New(ctx, db)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}

	token, err := generateToken()
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	tokenHash := store.HashToken(token)
	if err := st.RegisterDevice(ctx, id, name, deviceType, tokenHash, time.Now().UTC()); err != nil {
		return fmt.Errorf("register device: %w", err)
	}

	fmt.Printf("Device registered. This token is shown ONCE — copy it now:\n\n")
	fmt.Printf("  token: %s\n\n", token)
	fmt.Printf("Client config (config.yaml):\n\n")
	fmt.Printf("  server_url: %s\n  device_id: %s\n  token: %s\n  interval: 10s\n", serverURL, id, token)
	return nil
}

// generateToken returns a 32-byte random token encoded as 64 hex chars.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
