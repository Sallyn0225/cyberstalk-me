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