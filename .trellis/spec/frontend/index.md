# Frontend Development Guidelines

> Best practices for frontend development in this project.

---

## Overview

Guidelines for the React + Vite + TypeScript SPAs in this repo:

- `web/` — the public read-only page (device card grid, snapshot + SSE).
- `client-windows/webui/` — the `agent.exe -setup` configuration UI, added
  2026-07-30. Same stack and conventions; see
  [Directory Structure](./directory-structure.md) for what the two deliberately
  do not share, and why the setup UI must not re-implement any agent logic.

Greenfield note: these specs were seeded on 2026-07-28 from the confirmed tech
decisions in `.trellis/tasks/07-28-cyberstalk-me/design.md`. Code examples are
canonical patterns to follow, not extracts from existing code. When real code
diverges for a good reason, update the spec in the same task.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Flat `web/src` layout, naming | Filled |
| [Component Guidelines](./component-guidelines.md) | shadcn/ui-first components, Tailwind, lucide icons, Framer Motion, a11y | Filled |
| [Hook Guidelines](./hook-guidelines.md) | `useDeviceStream` contract, SSE lifecycle | Filled |
| [State Management](./state-management.md) | No state library; one hook owns server state | Filled |
| [Quality Guidelines](./quality-guidelines.md) | Lint/type gates, dependency policy, testing | Filled |
| [Type Safety](./type-safety.md) | `types/contract.ts` mirror of Go structs, parse guards | Filled |

Core invariant: the frontend is **read-only** and `src/types/contract.ts` must
stay in sync with the `shared/` Go module in the same task that changes either.

---

**Language**: All documentation should be written in **English**.
