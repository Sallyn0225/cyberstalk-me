# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

Guidelines for the Go backend (`server/`), the Go Windows client
(`client-windows/`), and the shared contract module (`shared/`).

Greenfield note: these specs were seeded on 2026-07-28 from the confirmed tech
decisions in `.trellis/tasks/07-28-cyberstalk-me/design.md` (Go + chi + SQLite
via `modernc.org/sqlite` + SSE, no CGO). Code examples are canonical patterns to
follow, not extracts from existing code. When real code diverges for a good
reason, update the spec in the same task.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Monorepo layout, Go workspace, package boundaries | Filled |
| [Database Guidelines](./database-guidelines.md) | SQLite/`database/sql` patterns, schema, upserts | Filled |
| [Error Handling](./error-handling.md) | Error wrapping, sentinels, API error shape | Filled |
| [Quality Guidelines](./quality-guidelines.md) | Tooling gates, forbidden patterns, testing | Filled |
| [Logging Guidelines](./logging-guidelines.md) | `log/slog` usage, levels, privacy rules | Filled |
| [Deployment Guidelines](./deployment-guidelines.md) | Container image invariants, compose deployment, CI/CD gates | Filled |

Cross-cutting rule worth repeating: **sanitization is the product's security
model** — raw window titles, device tokens, and visitor data never enter logs,
storage, or the wire contract.

That rule has exactly one exception, added 2026-07-30 by `07-30-config-webui`,
and it is deliberately narrow: **`agent.exe -setup` may show raw window titles
to the person sitting at the machine, over loopback HTTP.** Without it there is
no way to write title rules while looking at the titles they have to match.

The exception is bounded by seven constraints. Anything that touches the setup
path must keep all seven true:

| Constraint | Mechanism | Enforced by |
|------------|-----------|-------------|
| Never leaves the machine | `net.Listen("tcp", "127.0.0.1:0")`, hardcoded, no configurable bind address | `TestLiveSessionBindsLoopbackOnly` |
| No other local program can read it | One-time 32-byte `crypto/rand` token, required on every `/api/` route, compared with `subtle.ConstantTimeCompare` | `TestGuardRequiresTheSessionToken` |
| No page on another origin can read it | `Origin` + `Host` allowlist on the API *and* on the index page (the page hands out the token, so it needs the check too) | `TestGuardRejectsForeignOriginsAndHosts`, `TestIndexRejectsAReboundHost` |
| Never persisted | Titles live only in `setup.Catalog` (memory); `config.Marshal` serializes only `Config` fields | `TestLiveSessionSavesAConfigFromScratch` greps the written file |
| Never logged | No `slog` call in `setup`/`winsetup` takes a title, a body, or a token | review; the HTTP layer logs method+path only |
| Shortest possible lifetime | Exits on "finish", plus a 30-minute idle timeout | `TestIdleTimeoutEndsTheSession` |
| Cannot become an upload path | `internal/winsetup` does not import `internal/report`, and neither does anything it depends on | `TestSetupModeCannotReport` (runs `go list -deps`) |

The last row is the one to preserve most carefully: it makes "setup mode cannot
report anything" a property of the dependency graph rather than a promise about
code that someone could later break without noticing.

---

**Language**: All documentation should be written in **English**.
