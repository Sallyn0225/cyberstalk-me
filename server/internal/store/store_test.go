package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cyberstalk.me/shared"
)

// newTestStore returns a Store backed by an in-memory SQLite database with
// the schema applied. Uses shared cache so the single connection pool sees
// the same database.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	// Pool of 1 avoids "database is locked" in tests.
	db.SetMaxOpenConns(1)
	s, err := New(context.Background(), db)
	if err != nil {
		t.Fatalf("init store: %v", err)
	}
	return s
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestRegisterAndLookupByTokenHash(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	hash := sha256hex("secret-token")
	if err := s.RegisterDevice(ctx, "win-1", "My PC", "windows", hash, time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}

	dev, err := s.LookupByTokenHash(ctx, hash)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if dev.DeviceID != "win-1" || dev.DeviceName != "My PC" || dev.DeviceType != "windows" {
		t.Fatalf("unexpected device: %+v", dev)
	}
	if dev.TokenHash != hash {
		t.Fatalf("token hash mismatch")
	}
}

func TestLookupByTokenHashNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.LookupByTokenHash(context.Background(), sha256hex("nope"))
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("want ErrDeviceNotFound, got %v", err)
	}
}

func TestUpsertAndListStates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.RegisterDevice(ctx, "win-1", "My PC", "windows", sha256hex("tok-1"), time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := s.RegisterDevice(ctx, "and-1", "My Phone", "android", sha256hex("tok-2"), time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}

	net := "wifi"
	p1 := shared.ReportPayload{
		DeviceID:   "win-1",
		DeviceName: "My PC",
		DeviceType: "windows",
		Activity:   shared.Activity{App: "VS Code", Description: "writing code", Idle: false},
		Battery:    &shared.Battery{Level: ptrInt(82), Charging: true},
		Network:    &net,
		ReportedAt: time.Date(2026, 7, 28, 10, 30, 0, 0, time.UTC),
	}
	payloadJSON := jsonMarshal(t, p1)
	if err := s.UpsertState(ctx, "win-1", payloadJSON, p1.ReportedAt, p1.ReportedAt.Add(2*time.Second)); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}

	rows, err := s.ListStates(ctx)
	if err != nil {
		t.Fatalf("list states: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 state row, got %d", len(rows))
	}
	if rows[0].DeviceID != "win-1" || rows[0].Payload.Activity.App != "VS Code" {
		t.Fatalf("unexpected row: %+v", rows[0])
	}

	// Second device reports.
	p2 := shared.ReportPayload{
		DeviceID:   "and-1",
		DeviceName: "My Phone",
		DeviceType: "android",
		Activity:   shared.Activity{App: "Chrome", Description: "browsing", Idle: false},
		Network:    &net,
		ReportedAt: time.Date(2026, 7, 28, 10, 31, 0, 0, time.UTC),
	}
	if err := s.UpsertState(ctx, "and-1", jsonMarshal(t, p2), p2.ReportedAt, p2.ReportedAt); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	rows, err = s.ListStates(ctx)
	if err != nil {
		t.Fatalf("list states 2: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 state rows, got %d", len(rows))
	}

	// Upsert overwrites rather than accumulating (latest-state-only).
	p1Updated := p1
	p1Updated.Activity.App = "IntelliJ"
	if err := s.UpsertState(ctx, "win-1", jsonMarshal(t, p1Updated), p1Updated.ReportedAt, p1Updated.ReportedAt.Add(3*time.Second)); err != nil {
		t.Fatalf("upsert overwrite: %v", err)
	}
	rows, _ = s.ListStates(ctx)
	if len(rows) != 2 {
		t.Fatalf("upsert should overwrite, not add; got %d rows", len(rows))
	}
	for _, r := range rows {
		if r.DeviceID == "win-1" && r.Payload.Activity.App != "IntelliJ" {
			t.Fatalf("upsert did not overwrite: %+v", r)
		}
	}
}

func TestGetStateNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetState(context.Background(), "missing")
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("want ErrDeviceNotFound, got %v", err)
	}
}

func TestTokenNotStoredPlaintext(t *testing.T) {
	// Security red-line: the raw token must never appear in the database.
	// We verify by querying the devices table directly and checking that
	// token_hash is the hash, not the plaintext.
	s := newTestStore(t)
	ctx := context.Background()
	raw := "super-secret-token-xyz"
	hash := HashToken(raw)
	if err := s.RegisterDevice(ctx, "d1", "name", "windows", hash, time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}
	var storedHash, storedName string
	err := s.db.QueryRowContext(ctx, `SELECT token_hash, device_name FROM devices WHERE device_id = ?`, "d1").Scan(&storedHash, &storedName)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if storedHash != hash {
		t.Fatalf("stored hash mismatch: got %s want %s", storedHash, hash)
	}
	if storedHash == raw {
		t.Fatalf("plaintext token was stored in token_hash column")
	}
}

func TestSetLastSeenUpdatesRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterDevice(ctx, "d1", "n", "windows", sha256hex("t"), time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}
	old := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	if err := s.UpsertState(ctx, "d1", jsonMarshal(t, shared.ReportPayload{DeviceID: "d1", DeviceType: "windows", Activity: shared.Activity{App: "a"}}), old, old); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	stale := old.Add(-5 * time.Minute)
	if err := s.SetLastSeen(ctx, "d1", stale); err != nil {
		t.Fatalf("set last seen: %v", err)
	}
	row, err := s.GetState(ctx, "d1")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if !row.LastSeenAt.Equal(stale) {
		t.Fatalf("last seen not updated: got %v want %v", row.LastSeenAt, stale)
	}
}

func ptrInt(v int) *int { return &v }

// jsonMarshal marshals p for test payloads. json.Marshal of a ReportPayload
// cannot fail in practice, but t.Fatal keeps the test honest instead of
// panicking (spec: no panic outside main.go).
func jsonMarshal(t *testing.T, p shared.ReportPayload) []byte {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}
