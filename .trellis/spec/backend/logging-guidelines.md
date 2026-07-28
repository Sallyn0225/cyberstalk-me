# Logging Guidelines

> Structured logging conventions for the Go backend and Windows client.

---

## Stack

- Stdlib **`log/slog`** with the JSON handler on the server
  (`slog.NewJSONHandler(os.Stdout, ...)`); text handler is fine for the Windows
  client's local log file.
- Logger configured once in `main.go` and set as default via `slog.SetDefault`.
  Packages call `slog.Info(...)` etc. directly — no logger threading through
  constructors for this project size.
- Log to stdout only; the VPS process manager (systemd) owns persistence and
  rotation.

---

## Log Levels

| Level | Use for |
|-------|---------|
| `Debug` | Per-report details, SSE subscribe/unsubscribe. Off in production. |
| `Info` | Startup/shutdown, config summary, device online/offline transitions. |
| `Warn` | Rejected reports (bad token, unknown device), client retry/backoff. |
| `Error` | DB failures, unexpected handler errors — things a human should look at. |

The default production level is `Info`.

---

## Structured Fields

Always key-value attrs, never `fmt.Sprintf` into the message:

```go
slog.Info("device offline", "device_id", id, "last_seen", lastSeen)
slog.Warn("report rejected", "device_id", id, "reason", "bad token")
```

Standard keys: `device_id`, `reason`, `err` (use `"err", err`), `subscribers`.
Message strings are short, lowercase, stable (they get grepped).

---

## What NOT to Log — privacy is a product requirement

This site is fully public and the whole design rests on sanitization. Never log:

- **Device tokens** (raw or hashed) or `Authorization` header contents.
- **Raw window titles or process paths** on the client side — the raw title
  must never leave the device, and that includes the agent's own log file.
  Log only the mapped `{app, description}` result, and only at `Debug`.
- Visitor IPs or request headers on the public endpoints (snapshot/stream).
- Full report payloads at `Info` or above.

---

## What to Log

- Server: startup (port, offline threshold), each online/offline transition,
  auth rejections with reason, DB errors, SSE subscriber count changes (Debug).
- Windows client: startup (server URL, interval, rule count), report failures
  with backoff delay (Warn), mapping rule reload if implemented.
