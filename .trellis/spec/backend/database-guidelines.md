# Database Guidelines

> SQLite usage patterns for the backend.

---

## Stack

- **SQLite** via `modernc.org/sqlite` (pure Go, no CGO) + stdlib `database/sql`.
- **No ORM.** Hand-written SQL through prepared statements. The schema is two
  tables; an ORM adds nothing.
- One `*sql.DB` created in `cmd/server/main.go`, injected into `store` via a
  constructor (`store.New(db)`).

---

## Schema

Owned by `server/internal/store/schema.sql`, embedded with `go:embed` and
applied at startup with `CREATE TABLE IF NOT EXISTS`. No migration tool for the
MVP; schema changes are additive (`ALTER TABLE ... ADD COLUMN`) applied the same
way, guarded by `PRAGMA user_version`.

```sql
CREATE TABLE IF NOT EXISTS devices (
    device_id   TEXT PRIMARY KEY,
    device_name TEXT NOT NULL,
    device_type TEXT NOT NULL,          -- 'windows' | 'android'
    token_hash  TEXT NOT NULL,          -- SHA-256 hex of the device token
    created_at  TEXT NOT NULL           -- RFC 3339 UTC
);

CREATE TABLE IF NOT EXISTS device_state (
    device_id        TEXT PRIMARY KEY REFERENCES devices(device_id),
    last_report_json TEXT NOT NULL,     -- serialized shared.ReportPayload (sanitized)
    reported_at      TEXT NOT NULL,     -- RFC 3339 UTC, client clock
    last_seen_at     TEXT NOT NULL      -- RFC 3339 UTC, server clock
);
```

---

## Conventions

- Naming: `snake_case` tables and columns, singular column names, `_at` suffix
  for timestamps. Timestamps are stored as **RFC 3339 strings in UTC**.
- `device_state` is **latest-state-only**: every report is an upsert
  (`INSERT ... ON CONFLICT(device_id) DO UPDATE`). Never accumulate history
  rows — this is a product decision (no activity history is retained).
- Tokens are stored **only as SHA-256 hashes**, never plaintext.
- `last_report_json` stores the already-sanitized payload verbatim. The store
  layer must never receive raw window titles; sanitization happens on devices.
- All store methods take `context.Context` as the first parameter and use the
  `QueryRowContext` / `ExecContext` variants.
- Set `PRAGMA journal_mode=WAL` and `PRAGMA busy_timeout=5000` at startup;
  SQLite is single-writer and the SSE readers must not block reports.

---

## Canonical upsert example

```go
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
```

---

## Anti-patterns

- No ORMs, query builders, or migration frameworks.
- No `SELECT *` — always name columns so schema additions don't break scans.
- Do not open per-request DB connections; use the shared `*sql.DB` pool.
- Do not store anything user-identifying beyond the sanitized payload — no IPs,
  no raw titles, no request headers.
