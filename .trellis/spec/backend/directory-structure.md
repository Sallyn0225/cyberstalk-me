# Directory Structure

> How backend (Go) code is organized in this project.

---

## Overview

This is a greenfield monorepo. The layout below was fixed by the project design
(`.trellis/tasks/07-28-cyberstalk-me/design.md`) and is the source of truth for
where new Go code goes. Update this file if the real layout diverges once code
lands.

The repo hosts three Go modules tied together by a `go.work` workspace, plus the
frontend:

```
cyberstalk-me/
├── go.work                  # Go workspace: shared, server, client-windows
├── shared/                  # Go module: contract structs only (no logic)
│   └── contract.go          # ReportPayload, DeviceState, Activity, Battery, SSE event types
├── server/                  # Go module: backend (single static binary)
│   ├── cmd/server/main.go   # entrypoint: config load, wiring, http.ListenAndServe
│   └── internal/
│       ├── api/             # HTTP handlers, router (chi), SSE endpoint
│       ├── hub/             # SSE broadcast hub (subscriber set, fan-out)
│       ├── state/           # online/offline tracker (last_seen, ticker scan)
│       ├── store/           # SQLite persistence (devices, device_state)
│       └── config/          # env-based configuration
├── client-windows/          # Go module: Windows reporter agent (single exe)
│   ├── cmd/agent/
│   │   ├── main.go
│   │   ├── webui.go         # go:embed of the built setup UI + session-token injection
│   │   └── webui/           # COMMITTED build output of webui/ (embedded; CI diffs it)
│   ├── webui/               # setup UI source (React + Vite); see frontend spec
│   └── internal/
│       ├── collect/         # Win32 collectors (foreground window, idle, battery, network)
│       ├── config/          # config.yaml load + save + validation + defaults (pure logic, tested)
│       ├── mapping/         # sanitization rules: process name -> {app, description}
│       ├── report/          # HTTP POST loop with retry/backoff
│       ├── setup/           # -setup: catalog, draft, HTTP API + guards (pure, no Win32)
│       └── winsetup/        # -setup wiring: Win32 -> setup (windows-only, never imports report)
├── web/                     # React + Vite + TS frontend (see frontend spec)
└── client-android/          # Kotlin app (deferred, later child task)
```

---

## Module Organization

- **`shared/` contains only data types** used by both server and Windows client
  (request/response payloads, SSE event structs). No business logic, no
  third-party imports beyond stdlib. This keeps it importable from any module
  without dependency bleed.
- **`server/internal/` packages are organized by responsibility, not by layer
  ceremony.** Four packages (`api`, `hub`, `state`, `store`) cover the whole MVP;
  do not add `service/`, `repository/`, `dto/` style layers on top.
- **All packages under `internal/`** except `shared/` — nothing else is meant to
  be imported across modules.
- Dependency direction: `api` → (`hub`, `state`, `store`, `shared`); `hub`,
  `state`, `store` do not import `api` and do not import each other except
  `state` → `hub` (offline events are broadcast).
- Wiring (constructing store, hub, tracker, router) happens in
  `cmd/server/main.go`, not in package `init()`.
- **`client-windows/internal/` packages are isolated by design.** `config`,
  `collect`, `mapping`, and `report` do not import each other; `report` only
  receives `shared.ReportPayload`, and `mapping` only receives a process name
  plus a lazy title getter. Collect → mapping → payload assembly happens in
  `cmd/agent/main.go`. `collect` is Windows-only (`//go:build windows`) with no
  cross-platform stub; the other three are pure/portable and unit-tested.
- **`setup` is pure; `winsetup` is the only Windows-only part of setup mode.**
  `setup` must not import `collect`, and has no build tag at all: the foreground
  reading is injected as a `setup.Source func() setup.Foreground`, whose Win32
  implementation lives in `winsetup`. This is what lets the catalog, the draft
  logic and every HTTP handler be tested on any platform, including a Linux
  runner. Adding a `//go:build windows` file to `setup` gives that up.
- **`winsetup` must never import `report`, directly or transitively.** That is
  the structural half of "setup mode cannot phone home" (see the exception table
  in [index.md](./index.md)), and `TestSetupModeCannotReport` asserts it with
  `go list -deps`. If setup mode ever needs to talk to the server, that is a
  design decision to make explicitly, not a new import to add quietly.

---

## Naming Conventions

- Package names: short, lowercase, singular (`store`, `hub`, `mapping`).
- Files: lowercase with underscores only when needed (`sse.go`, `report_handler.go`).
- One file per handler group is fine; do not create one file per function.
- Test files sit next to the code: `store_test.go` beside `store.go`.

---

## Anti-patterns

- No `pkg/` directory — use `internal/` (or `shared/` for cross-module types).
- No `utils`/`helpers` grab-bag packages. If a helper has no home, it probably
  belongs next to its only caller.
- No CGO anywhere (SQLite driver is `modernc.org/sqlite`); the build must stay
  `CGO_ENABLED=0` cross-compilable from Windows to Linux.
