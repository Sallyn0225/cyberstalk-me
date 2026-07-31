# Error Handling

> How errors are handled in the Go backend and Windows client.

---

## Principles

- Standard Go error handling: return `error`, wrap with context using
  `fmt.Errorf("doing X for %s: %w", id, err)`, check with `errors.Is` /
  `errors.As`. No custom error framework.
- Errors are **logged once**, at the top of the call chain (HTTP handler or the
  client's report loop). Lower layers wrap and return; they do not log.
- `panic` is never used for control flow. Handlers run behind chi's `Recoverer`
  middleware as a last resort only.

---

## Sentinel Errors

Define sentinels in the package that owns the condition:

```go
// server/internal/store/store.go
var ErrDeviceNotFound = errors.New("device not found")

// server/internal/api/auth.go
var ErrBadToken = errors.New("invalid device token")
```

Handlers map sentinels to HTTP status codes with `errors.Is`; unknown errors
become 500.

---

## API Error Responses

All error responses are JSON with a single stable shape:

```json
{ "error": "invalid device token" }
```

Status code mapping:

| Condition | Status |
|-----------|--------|
| Malformed JSON body / missing required fields | 400 |
| Missing or invalid `Authorization: Bearer` token, or body `device_id` mismatch with the token-bound device | 401 |
| Anything unexpected | 500 (body is the generic `"internal error"` — never the wrapped error text) |

There is no 404 on the report endpoint: an unknown token and an unknown
device both collapse into the 401 `"invalid device token"` at the auth layer
(`ErrDeviceNotFound` is never surfaced as 404).

Write them through one helper so the shape can't drift:

```go
func writeError(w http.ResponseWriter, status int, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

---

## Client (Windows agent) Errors

- A failed report POST is **not fatal**: log a warning, retry with exponential
  backoff (cap ~2 minutes), keep the loop alive. The agent must survive server
  restarts and network drops indefinitely.
- Collector failures (a Win32 call errors) degrade the field to its zero/null
  value for that cycle instead of skipping the report.

---

## Common Mistakes to Avoid

- Returning the internal error string in a 500 body — it can leak paths or SQL.
- Logging the same error at every layer (log once at the top).
- Wrapping with no added context (`fmt.Errorf("%w", err)` adds nothing — say
  what was being attempted).
- Swallowing errors from `json.NewEncoder(...).Encode` in SSE writes — a write
  error means the subscriber is gone and must be unsubscribed.
