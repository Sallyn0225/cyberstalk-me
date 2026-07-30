package main

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberstalk.me/server/internal/store"

	_ "modernc.org/sqlite" // register sqlite driver for tests
)

// TestRunRegisterDevice exercises the register-device admin CLI end to end:
// it runs the command against a temporary on-disk SQLite database, checks the
// one-time token + client config are printed to stdout, and verifies the
// device is persisted with a SHA-256 hash (never the plaintext token).
func TestRunRegisterDevice(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	args := []string{"d1", "My PC", "windows", "--sqlite-path", dbPath, "--server-url", "http://example:8080"}

	// Capture stdout: runRegister prints the one-time token via fmt.Printf.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	runErr := runRegister(args)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if runErr != nil {
		t.Fatalf("runRegister: %v", runErr)
	}
	s := string(out)
	if !strings.Contains(s, "token:") {
		t.Errorf("stdout missing 'token:' line\n---stdout---\n%s", s)
	}
	if !strings.Contains(s, "device_id: d1") {
		t.Errorf("stdout missing 'device_id: d1'\n---stdout---\n%s", s)
	}
	if !strings.Contains(s, "server_url: http://example:8080") {
		t.Errorf("stdout missing 'server_url'\n---stdout---\n%s", s)
	}

	// The device must be in the DB with a hash, and the printed plaintext
	// token must hash to that stored value (and not equal it).
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	st, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatalf("init store: %v", err)
	}
	dev, err := st.LookupDevice(context.Background(), "d1")
	if err != nil {
		t.Fatalf("lookup device: %v", err)
	}
	if dev.DeviceName != "My PC" || dev.DeviceType != "windows" {
		t.Fatalf("unexpected device: %+v", dev)
	}
	if len(dev.TokenHash) != 64 {
		t.Fatalf("token_hash want 64 hex chars (sha256), got %d", len(dev.TokenHash))
	}

	// Extract the printed plaintext token and relate it to the stored hash.
	var printed string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "token:") {
			printed = strings.TrimSpace(strings.TrimPrefix(line, "token:"))
		}
	}
	if printed == "" {
		t.Fatalf("could not extract token from stdout\n%s", s)
	}
	if printed == dev.TokenHash {
		t.Fatalf("plaintext token was stored as-is (must be hashed)")
	}
	if store.HashToken(printed) != dev.TokenHash {
		t.Fatalf("printed token does not hash to stored token_hash")
	}
}

// TestPruneUsageRunsAlongsideWritesAndStopsOnCancel exercises the retention
// goroutine the way it actually runs: sweeping on a tick while reports are
// being written. SQLite has a single writer, so this is the one place where
// housekeeping and the hot path contend — worth running under -race.
func TestPruneUsageRunsAlongsideWritesAndStopsOnCancel(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "prune.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithCancel(context.Background())
	st, err := store.New(ctx, db)
	if err != nil {
		t.Fatalf("init store: %v", err)
	}
	if err := st.RegisterDevice(ctx, "d1", "PC", "windows", store.HashToken("tok"), time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Old buckets that a zero-day retention window must remove, and a fresh
	// one that it must keep.
	old := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	now := time.Now().UTC().Truncate(time.Hour)
	if err := st.AddUsage(ctx, "d1", []store.UsageDelta{
		{HourStart: old, State: "active", App: "Code", Description: "写代码", Seconds: 60},
		{HourStart: now, State: "active", App: "Code", Description: "写代码", Seconds: 60},
	}); err != nil {
		t.Fatalf("seed usage: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		pruneUsage(ctx, st, 1, time.Millisecond)
	}()

	// Keep writing while the sweep runs.
	for i := 0; i < 50; i++ {
		if err := st.AddUsage(ctx, "d1", []store.UsageDelta{
			{HourStart: now, State: "active", App: "Code", Description: "写代码", Seconds: 1},
		}); err != nil {
			t.Fatalf("concurrent add usage: %v", err)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pruneUsage did not return after ctx cancel")
	}

	rows, err := st.QueryUsage(context.Background(), old, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("query usage: %v", err)
	}
	// The 48-hour-old bucket is outside a one-day retention window; the
	// current hour is inside it.
	for _, r := range rows {
		if r.HourStart.Equal(old) {
			t.Fatalf("old bucket survived pruning: %+v", r)
		}
	}
	if len(rows) != 1 || !rows[0].HourStart.Equal(now) {
		t.Fatalf("remaining rows = %+v, want only the current hour", rows)
	}
}

// TestRunRegisterDeviceErrors covers the argument-validation error paths.
// These return before any token is printed, so no stdout capture is needed.
func TestRunRegisterDeviceErrors(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cases := []struct {
		name string
		args []string
	}{
		{"wrong arg count", []string{"d1", "windows", "--sqlite-path", dbPath}},
		{"bad device type", []string{"d1", "n", "ios", "--sqlite-path", dbPath}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRegister(tc.args); err == nil {
				t.Fatalf("want error, got nil")
			}
		})
	}
}
