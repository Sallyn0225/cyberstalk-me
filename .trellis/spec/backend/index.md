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

Cross-cutting rule worth repeating: **sanitization is the product's security
model** — raw window titles, device tokens, and visitor data never enter logs,
storage, or the wire contract.

---

**Language**: All documentation should be written in **English**.
