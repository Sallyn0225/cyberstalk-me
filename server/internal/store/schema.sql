-- Schema for the cyberstalk-me backend. Embedded at startup and applied
-- with CREATE TABLE IF NOT EXISTS. No migration tool for the MVP; additive
-- changes (ALTER TABLE ... ADD COLUMN) are guarded by PRAGMA user_version.

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

-- Hourly usage buckets. This table intentionally accumulates rows, unlike
-- device_state: it is the aggregate the screen-time view reads. It is NOT raw
-- report history — one row is "device D spent N seconds in state S on app A
-- doing D' during UTC hour H", and individual reports are never retained.
-- Bounded by USAGE_RETENTION_DAYS (see PruneUsage).
CREATE TABLE IF NOT EXISTS usage_bucket (
    device_id   TEXT    NOT NULL REFERENCES devices(device_id),
    hour_start  TEXT    NOT NULL,       -- RFC 3339 UTC, truncated to the hour
    state       TEXT    NOT NULL,       -- 'active' | 'idle' | 'locked'
    app         TEXT    NOT NULL,
    description TEXT    NOT NULL,
    seconds     INTEGER NOT NULL,
    PRIMARY KEY (device_id, hour_start, state, app, description)
);

-- The primary key leads with device_id, so retention pruning (WHERE
-- hour_start < ?) cannot use it. This index is what keeps pruning cheap.
CREATE INDEX IF NOT EXISTS idx_usage_bucket_hour_start
    ON usage_bucket(hour_start);