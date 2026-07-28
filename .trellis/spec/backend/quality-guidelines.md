# Quality Guidelines

> Code quality standards for the Go backend and Windows client.

---

## Tooling (must pass before any commit)

```bash
gofmt -l .          # no output = formatted
go vet ./...
go test ./...
go build ./...      # with CGO_ENABLED=0
```

No golangci-lint requirement for the MVP; `gofmt` + `go vet` + tests is the
bar. If golangci-lint is added later, update this file with the chosen config.

---

## Required Patterns

- `context.Context` first parameter on anything that does I/O.
- Constructor injection (`store.New(db)`, `hub.New()`); wiring lives in
  `cmd/*/main.go`.
- Contract types come from the `shared/` module — never redeclare payload
  structs inside `server/` or `client-windows/`.
- Concurrency: the SSE hub owns its subscriber map behind a mutex; all
  cross-goroutine communication goes through channels or the hub. Guard shared
  maps — the race detector (`go test -race`) must stay clean.
- Graceful shutdown: `signal.NotifyContext` + `http.Server.Shutdown` so SSE
  connections close cleanly on deploy.

---

## Forbidden Patterns

- **CGO** — everything must cross-compile from Windows to Linux with
  `CGO_ENABLED=0 GOOS=linux go build`.
- `panic`/`log.Fatal` outside `main.go`.
- Package-level mutable state (except the default slog logger).
- Adding heavyweight dependencies for things the stdlib covers. Approved
  third-party list for the MVP: `github.com/go-chi/chi/v5`,
  `modernc.org/sqlite`, `gopkg.in/yaml.v3`, `golang.org/x/sys`. Additions need
  a reason recorded in the task PRD or this file.
- Accepting or persisting unsanitized fields: the server-side contract has no
  place for raw window titles; do not add one.

---

## Testing Requirements

- Table-driven tests with the stdlib `testing` package; no assertion libraries.
- HTTP handlers tested with `net/http/httptest` against a real in-memory
  SQLite store (`:memory:` DSN) — no store mocks.
- Minimum for the MVP: auth rejection paths (bad/missing token), report upsert,
  snapshot shape, and the offline-threshold transition (inject a fake clock
  into the state tracker: it takes a `now func() time.Time`).
- Win32 collectors (`client-windows/internal/collect`) are exempt from unit
  tests; the mapping package (`internal/mapping`) is pure and **must** be
  tested, including the "unknown process falls back to generic description"
  rule — that's the privacy boundary.

---

## Code Review Checklist

- [ ] `gofmt` / `go vet` / `go test -race` clean, `CGO_ENABLED=0` build works
- [ ] Errors wrapped with context, logged once at the top
- [ ] No token, raw title, or visitor data in logs or storage
- [ ] Contract changes made in `shared/` and mirrored in `web/src/types/`
- [ ] New SQL uses placeholders (`?`), never string concatenation
