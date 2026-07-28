package main

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
