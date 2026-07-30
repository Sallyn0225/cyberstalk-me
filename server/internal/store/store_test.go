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

// usageHour is a UTC hour-aligned timestamp for the usage tests.
func usageHour(hour int) time.Time {
	return time.Date(2026, 7, 30, hour, 0, 0, 0, time.UTC)
}

// findUsage returns the seconds stored for one bucket key, and whether the
// bucket exists at all.
func findUsage(rows []UsageRow, deviceID string, hour int, state, app, description string) (int, bool) {
	for _, r := range rows {
		if r.DeviceID == deviceID && r.HourStart.Equal(usageHour(hour)) &&
			r.State == state && r.App == app && r.Description == description {
			return r.Seconds, true
		}
	}
	return 0, false
}

func TestAddUsageAccumulatesInsteadOfOverwriting(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterDevice(ctx, "usage-add", "PC", "windows", sha256hex("t-usage-add"), time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}

	delta := UsageDelta{HourStart: usageHour(13), State: "active", App: "Code", Description: "写代码", Seconds: 10}
	if err := s.AddUsage(ctx, "usage-add", []UsageDelta{delta}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	// The same key again must add, not replace: two reports in the same hour
	// on the same activity are two separate stretches of time.
	if err := s.AddUsage(ctx, "usage-add", []UsageDelta{delta}); err != nil {
		t.Fatalf("second add: %v", err)
	}
	// A different description is a different bucket.
	other := delta
	other.Description = "看文档"
	other.Seconds = 4
	if err := s.AddUsage(ctx, "usage-add", []UsageDelta{other}); err != nil {
		t.Fatalf("third add: %v", err)
	}

	rows, err := s.QueryUsage(ctx, usageHour(0), usageHour(23))
	if err != nil {
		t.Fatalf("query usage: %v", err)
	}
	if got, ok := findUsage(rows, "usage-add", 13, "active", "Code", "写代码"); !ok || got != 20 {
		t.Fatalf("accumulated seconds = %d (found %v), want 20", got, ok)
	}
	if got, ok := findUsage(rows, "usage-add", 13, "active", "Code", "看文档"); !ok || got != 4 {
		t.Fatalf("second description seconds = %d (found %v), want 4", got, ok)
	}
}

func TestAddUsageEmptyIsNoOp(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddUsage(context.Background(), "nobody", nil); err != nil {
		t.Fatalf("empty add should not error: %v", err)
	}
}

func TestAddUsageWritesAllBucketsOfOneBatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterDevice(ctx, "usage-batch", "PC", "windows", sha256hex("t-usage-batch"), time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}
	// What an hour-crossing interval produces: one batch, two hours.
	batch := []UsageDelta{
		{HourStart: usageHour(13), State: "active", App: "Code", Description: "写代码", Seconds: 5},
		{HourStart: usageHour(14), State: "active", App: "Code", Description: "写代码", Seconds: 5},
	}
	if err := s.AddUsage(ctx, "usage-batch", batch); err != nil {
		t.Fatalf("add batch: %v", err)
	}
	rows, err := s.QueryUsage(ctx, usageHour(0), usageHour(23))
	if err != nil {
		t.Fatalf("query usage: %v", err)
	}
	for _, hour := range []int{13, 14} {
		if got, ok := findUsage(rows, "usage-batch", hour, "active", "Code", "写代码"); !ok || got != 5 {
			t.Fatalf("hour %d seconds = %d (found %v), want 5", hour, got, ok)
		}
	}
}

func TestQueryUsageRangeIsHalfOpen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterDevice(ctx, "usage-range", "PC", "windows", sha256hex("t-usage-range"), time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}
	for _, hour := range []int{12, 13, 14} {
		d := UsageDelta{HourStart: usageHour(hour), State: "active", App: "Code", Description: "写代码", Seconds: 60}
		if err := s.AddUsage(ctx, "usage-range", []UsageDelta{d}); err != nil {
			t.Fatalf("add hour %d: %v", hour, err)
		}
	}

	// [13:00, 14:00): the bucket equal to from is in, the one equal to to is out.
	rows, err := s.QueryUsage(ctx, usageHour(13), usageHour(14))
	if err != nil {
		t.Fatalf("query usage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	if !rows[0].HourStart.Equal(usageHour(13)) {
		t.Fatalf("row hour_start = %s, want %s", rows[0].HourStart, usageHour(13))
	}
	if rows[0].App != "Code" || rows[0].Description != "写代码" || rows[0].Seconds != 60 || rows[0].State != "active" {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
}

func TestQueryUsageSeparatesDevices(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"usage-iso-a", "usage-iso-b"} {
		if err := s.RegisterDevice(ctx, id, "PC", "windows", sha256hex("t-"+id), time.Now().UTC()); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	a := UsageDelta{HourStart: usageHour(13), State: "active", App: "Code", Description: "写代码", Seconds: 10}
	b := UsageDelta{HourStart: usageHour(13), State: "active", App: "Code", Description: "写代码", Seconds: 30}
	if err := s.AddUsage(ctx, "usage-iso-a", []UsageDelta{a}); err != nil {
		t.Fatalf("add a: %v", err)
	}
	if err := s.AddUsage(ctx, "usage-iso-b", []UsageDelta{b}); err != nil {
		t.Fatalf("add b: %v", err)
	}

	rows, err := s.QueryUsage(ctx, usageHour(13), usageHour(14))
	if err != nil {
		t.Fatalf("query usage: %v", err)
	}
	// Same bucket key on two devices must stay two rows — the device is part
	// of the primary key.
	if got, ok := findUsage(rows, "usage-iso-a", 13, "active", "Code", "写代码"); !ok || got != 10 {
		t.Fatalf("device a seconds = %d (found %v), want 10", got, ok)
	}
	if got, ok := findUsage(rows, "usage-iso-b", 13, "active", "Code", "写代码"); !ok || got != 30 {
		t.Fatalf("device b seconds = %d (found %v), want 30", got, ok)
	}
}

func TestPruneUsageDeletesOnlyOlderBuckets(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterDevice(ctx, "usage-prune", "PC", "windows", sha256hex("t-usage-prune"), time.Now().UTC()); err != nil {
		t.Fatalf("register: %v", err)
	}
	for _, hour := range []int{10, 11, 13} {
		d := UsageDelta{HourStart: usageHour(hour), State: "active", App: "Code", Description: "写代码", Seconds: 60}
		if err := s.AddUsage(ctx, "usage-prune", []UsageDelta{d}); err != nil {
			t.Fatalf("add hour %d: %v", hour, err)
		}
	}

	n, err := s.PruneUsage(ctx, usageHour(13))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 2 {
		t.Fatalf("pruned %d rows, want 2", n)
	}
	rows, err := s.QueryUsage(ctx, usageHour(0), usageHour(23))
	if err != nil {
		t.Fatalf("query usage: %v", err)
	}
	if len(rows) != 1 || !rows[0].HourStart.Equal(usageHour(13)) {
		t.Fatalf("remaining rows = %+v, want only the 13:00 bucket", rows)
	}

	// Pruning again removes nothing and must not error.
	n, err = s.PruneUsage(ctx, usageHour(13))
	if err != nil {
		t.Fatalf("second prune: %v", err)
	}
	if n != 0 {
		t.Fatalf("second prune removed %d rows, want 0", n)
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
