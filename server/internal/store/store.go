// Package store is the SQLite persistence layer.
//
// It owns two tables (devices, device_state) and exposes only what the
// rest of the server needs: device registration, token-hash lookup, and
// latest-state upsert/list/get. All methods take context.Context first and
// use the QueryRowContext/ExecContext variants. Tokens are stored only as
// SHA-256 hashes; the store never receives or persists raw device tokens or
// raw window titles.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed" // for schema embed
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"cyberstalk.me/shared"
)

// ErrDeviceNotFound is returned when a device_id is not present in the
// devices table. Callers map it to a 404 via errors.Is.
var ErrDeviceNotFound = errors.New("device not found")

//go:embed schema.sql
var schemaSQL string

// Store wraps a *sql.DB with the two tables. The DB is created and owned by
// main (in cmd/server/main.go) and injected here via New so tests can supply
// an in-memory database.
type Store struct {
	db *sql.DB
}

// New returns a Store over the given DB and applies the schema. It also sets
// the pragmas required for single-writer SQLite with non-blocking readers.
func New(ctx context.Context, db *sql.DB) (*Store, error) {
	// WAL keeps readers from blocking on a writer; busy_timeout lets the
	// writer wait briefly for a lock instead of erroring immediately.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return nil, fmt.Errorf("apply %s: %w", pragma, err)
		}
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// HashToken returns the lowercase hex SHA-256 of a raw device token. It is
// exported so the auth layer and the register-device CLI hash consistently.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RegisterDevice inserts a new device with the given token hash. It is used
// by the register-device admin CLI. created_at is the server's now.
func (s *Store) RegisterDevice(ctx context.Context, id, name, deviceType, tokenHash string, createdAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO devices (device_id, device_name, device_type, token_hash, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			device_name = excluded.device_name,
			device_type = excluded.device_type,
			token_hash  = excluded.token_hash`,
		id, name, deviceType, tokenHash, createdAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("register device %s: %w", id, err)
	}
	return nil
}

// Device is a row of the devices table.
type Device struct {
	DeviceID   string
	DeviceName string
	DeviceType string
	TokenHash  string
	CreatedAt  time.Time
}

// LookupByTokenHash returns the device bound to the given token hash, or
// ErrDeviceNotFound if no device has that hash.
func (s *Store) LookupByTokenHash(ctx context.Context, tokenHash string) (Device, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT device_id, device_name, device_type, token_hash, created_at
		FROM devices
		WHERE token_hash = ?`, tokenHash)
	var d Device
	var createdAt string
	if err := row.Scan(&d.DeviceID, &d.DeviceName, &d.DeviceType, &d.TokenHash, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Device{}, ErrDeviceNotFound
		}
		return Device{}, fmt.Errorf("lookup device by token hash: %w", err)
	}
	t, parseErr := time.Parse(time.RFC3339, createdAt)
	if parseErr != nil {
		return Device{}, fmt.Errorf("parse created_at for %s: %w", d.DeviceID, parseErr)
	}
	d.CreatedAt = t
	return d, nil
}

// LookupDevice returns the device row for id, or ErrDeviceNotFound.
func (s *Store) LookupDevice(ctx context.Context, id string) (Device, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT device_id, device_name, device_type, token_hash, created_at
		FROM devices
		WHERE device_id = ?`, id)
	var d Device
	var createdAt string
	if err := row.Scan(&d.DeviceID, &d.DeviceName, &d.DeviceType, &d.TokenHash, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Device{}, ErrDeviceNotFound
		}
		return Device{}, fmt.Errorf("lookup device %s: %w", id, err)
	}
	t, parseErr := time.Parse(time.RFC3339, createdAt)
	if parseErr != nil {
		return Device{}, fmt.Errorf("parse created_at for %s: %w", id, parseErr)
	}
	d.CreatedAt = t
	return d, nil
}

// ListDevices returns every registered device.
func (s *Store) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT device_id, device_name, device_type, token_hash, created_at
		FROM devices
		ORDER BY device_id`)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		var createdAt string
		if err := rows.Scan(&d.DeviceID, &d.DeviceName, &d.DeviceType, &d.TokenHash, &createdAt); err != nil {
			return nil, fmt.Errorf("scan device row: %w", err)
		}
		t, parseErr := time.Parse(time.RFC3339, createdAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse created_at: %w", parseErr)
		}
		d.CreatedAt = t
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device rows: %w", err)
	}
	return out, nil
}

// UpsertState stores the latest report payload for a device, overwriting any
// previous row. reportedAt is the client clock; seenAt is the server clock
// and is the basis for online/offline judgment.
func (s *Store) UpsertState(ctx context.Context, id string, payload []byte, reportedAt, seenAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO device_state (device_id, last_report_json, reported_at, last_seen_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			last_report_json = excluded.last_report_json,
			reported_at      = excluded.reported_at,
			last_seen_at     = excluded.last_seen_at`,
		id, string(payload), reportedAt.UTC().Format(time.RFC3339), seenAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert state for %s: %w", id, err)
	}
	return nil
}

// StateRow is a device's latest state plus the metadata needed to judge
// online/offline. Payload is the deserialized sanitized report.
type StateRow struct {
	DeviceID   string
	DeviceName string
	DeviceType string
	Payload    shared.ReportPayload
	ReportedAt time.Time
	LastSeenAt time.Time
}

// GetState returns the latest state row for id, or ErrDeviceNotFound if the
// device is registered but has never reported (or isn't registered at all).
func (s *Store) GetState(ctx context.Context, id string) (StateRow, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT d.device_id, d.device_name, d.device_type,
		       s.last_report_json, s.reported_at, s.last_seen_at
		FROM devices d
		LEFT JOIN device_state s ON s.device_id = d.device_id
		WHERE d.device_id = ?`, id)
	var sr StateRow
	var reportJSON sql.NullString
	var reportedAt, lastSeenAt string
	if err := row.Scan(&sr.DeviceID, &sr.DeviceName, &sr.DeviceType, &reportJSON, &reportedAt, &lastSeenAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StateRow{}, ErrDeviceNotFound
		}
		return StateRow{}, fmt.Errorf("get state for %s: %w", id, err)
	}
	if !reportJSON.Valid {
		// Registered but never reported: no payload yet. Treat as not found
		// for state purposes so the caller can skip it in lists.
		return StateRow{
			DeviceID:   sr.DeviceID,
			DeviceName: sr.DeviceName,
			DeviceType: sr.DeviceType,
		}, nil
	}
	if err := decodeJSON(reportJSON.String, &sr.Payload); err != nil {
		return StateRow{}, fmt.Errorf("decode payload for %s: %w", id, err)
	}
	rt, err := time.Parse(time.RFC3339, reportedAt)
	if err != nil {
		return StateRow{}, fmt.Errorf("parse reported_at for %s: %w", id, err)
	}
	st, err := time.Parse(time.RFC3339, lastSeenAt)
	if err != nil {
		return StateRow{}, fmt.Errorf("parse last_seen_at for %s: %w", id, err)
	}
	sr.ReportedAt = rt
	sr.LastSeenAt = st
	return sr, nil
}

// ListStates returns the latest state of every registered device that has
// reported at least once. Devices that registered but never reported are
// omitted (they have no activity to show).
func (s *Store) ListStates(ctx context.Context) ([]StateRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.device_id, d.device_name, d.device_type,
		       s.last_report_json, s.reported_at, s.last_seen_at
		FROM devices d
		JOIN device_state s ON s.device_id = d.device_id
		ORDER BY d.device_id`)
	if err != nil {
		return nil, fmt.Errorf("list states: %w", err)
	}
	defer rows.Close()
	var out []StateRow
	for rows.Next() {
		var sr StateRow
		var reportJSON, reportedAt, lastSeenAt string
		if err := rows.Scan(&sr.DeviceID, &sr.DeviceName, &sr.DeviceType, &reportJSON, &reportedAt, &lastSeenAt); err != nil {
			return nil, fmt.Errorf("scan state row: %w", err)
		}
		if err := decodeJSON(reportJSON, &sr.Payload); err != nil {
			return nil, fmt.Errorf("decode payload for %s: %w", sr.DeviceID, err)
		}
		rt, err := time.Parse(time.RFC3339, reportedAt)
		if err != nil {
			return nil, fmt.Errorf("parse reported_at for %s: %w", sr.DeviceID, err)
		}
		st, err := time.Parse(time.RFC3339, lastSeenAt)
		if err != nil {
			return nil, fmt.Errorf("parse last_seen_at for %s: %w", sr.DeviceID, err)
		}
		sr.ReportedAt = rt
		sr.LastSeenAt = st
		out = append(out, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate state rows: %w", err)
	}
	return out, nil
}

// ListDeviceStates returns the latest state of every registered device that
// has reported at least once, projected into the shared.DeviceState wire
// shape. The Online field is left zero (false); the caller (the state
// tracker or snapshot handler) judges online status from LastSeenAt using
// its own clock. This method satisfies state.StateLister so the state
// package never has to import store.
func (s *Store) ListDeviceStates(ctx context.Context) ([]shared.DeviceState, error) {
	rows, err := s.ListStates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]shared.DeviceState, len(rows))
	for i, r := range rows {
		out[i] = shared.DeviceState{
			DeviceID:   r.DeviceID,
			DeviceName: r.DeviceName,
			DeviceType: r.DeviceType,
			Activity:   r.Payload.Activity,
			Battery:    r.Payload.Battery,
			Network:    r.Payload.Network,
			ReportedAt: r.ReportedAt,
			LastSeenAt: r.LastSeenAt,
		}
	}
	return out, nil
}

// SetLastSeen updates last_seen_at for id without changing the report
// payload. It is used by tests to simulate stale devices. It is a no-op
// (returns ErrDeviceNotFound) if the device has never reported, because
// device_state has no row.
func (s *Store) SetLastSeen(ctx context.Context, id string, seenAt time.Time) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE device_state SET last_seen_at = ?
		WHERE device_id = ?`,
		seenAt.UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("set last_seen for %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected for %s: %w", id, err)
	}
	if n == 0 {
		return ErrDeviceNotFound
	}
	return nil
}
