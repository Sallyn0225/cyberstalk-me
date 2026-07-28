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
│   ├── cmd/agent/main.go
│   └── internal/
│       ├── collect/         # Win32 collectors (foreground window, idle, battery, network)
│       ├── mapping/         # sanitization rules: process name -> {app, description}
│       └── report/          # HTTP POST loop with retry/backoff
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
