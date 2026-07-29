# Quality Guidelines

> Code quality standards for the Go backend and Windows client.

---

## Tooling (must pass before any commit)

Run from the repo root with **explicit module paths**. The `go.work` workspace
makes bare `go build ./...` fail with "directory prefix . does not contain
modules listed in go.work", so the three modules must be listed by name:

```bash
gofmt -l shared server client-windows   # no output = formatted
go vet ./server/... ./shared/... ./client-windows/...
go test ./server/... ./shared/... ./client-windows/...
CGO_ENABLED=0 go build ./server/... ./shared/... ./client-windows/...
```

The Linux cross-compile gate (the VPS deploy check) covers only the server-side
modules — `client-windows` is Windows-only by build constraint, so `GOOS=linux`
excludes all of its files ("build constraints exclude all Go files") and it
cannot be cross-built to Linux:

```bash
CGO_ENABLED=0 GOOS=linux go build ./server/... ./shared/...
```

The Windows client is instead validated by the native-Windows gate above
(`./client-windows/...` in build/vet/test). Its Win32 collectors
(`internal/collect`) are Windows-only with no cross-platform stub — a stub
would only buy a fake green build for code that can never run anywhere else.

No golangci-lint requirement for the MVP; `gofmt` + `go vet` + tests is the
bar. If golangci-lint is added later, update this file with the chosen config.

The race detector (`go test -race`) is also required (see Code Review
Checklist). On Windows it needs a C compiler, because race support is built
on cgo: install mingw-w64 (e.g. a WinLibs UCRT build) and put its `bin` on
`PATH` so `gcc` is found, then run `CGO_ENABLED=1 go test -race ./...`.
The production build stays `CGO_ENABLED=0` — gcc is only needed for the
race-enabled test run, not for the shipped binary.

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
