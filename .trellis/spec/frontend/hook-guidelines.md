# Hook Guidelines

> How hooks are used in this project.

---

## Naming Conventions

- Files and exports: `useXxx` camelCase, one hook per file in `src/hooks/`.
- A hook returns a plain object with named fields, typed explicitly.

---

## Data Fetching

- **No React Query / SWR.** The app has two server-state sources — the live
  snapshot + SSE stream (`useDeviceStream`) and the usage aggregates
  (`useUsage`) — and neither needs caching, mutations or invalidation. A
  fetching library adds nothing here.
- Plain `fetch` for one-shot requests (snapshot, usage); `EventSource` for the
  stream.
- API paths are relative (`/api/v1/...`); Vite dev proxy handles local dev.
- Every one-shot fetch is aborted from the effect cleanup via `AbortController`,
  so a fast prop change cannot land an older response last.

---

## The canonical hook: `useDeviceStream`

All server communication is encapsulated here. Shape:

```ts
// hooks/useDeviceStream.ts
export type ConnectionState = 'connecting' | 'live' | 'reconnecting'

export interface DeviceStreamState {
  devices: DeviceState[] // sorted stable by device_id
  connection: ConnectionState
  error: string | null // initial snapshot failed; an SSE drop is not an error
}

export function useDeviceStream(): DeviceStreamState
```

Required behavior:

1. On mount: `GET /api/v1/snapshot` → full device list into state (bare array,
   see type-safety.md). Abort it from the effect cleanup via `AbortController`.
2. Then open `EventSource("/api/v1/stream")`; each event merges into the list
   keyed by `device_id` (update or insert, never duplicate).
3. **Cleanup on unmount**: `es.close()` in the effect cleanup — leaking
   EventSources is the classic bug here.
4. `onopen` is the readiness signal, **not** the first message: the server
   opens the stream with a *named* frame (`event: ready`), and named events
   never reach `onmessage`. On the first open set `connection: "live"`; on any
   later open (i.e. a reconnect) **also re-fetch the snapshot** — track that
   with a plain `let connectedOnce` inside the effect.
5. On `onerror`: set `connection: "reconnecting"` and rely on EventSource's
   built-in retry. Never construct a replacement EventSource there.
6. Parse events through the typed helpers in `types/contract.ts`
   (see type-safety.md); malformed events are dropped with a `console.warn`,
   they must not crash the page.

State merging uses `useReducer` when the update logic grows beyond a single
`setDevices` call; start with `useState` and refactor when needed.

---

## The second hook: `useUsage`

```ts
// hooks/useUsage.ts
export interface UsageState {
  data: UsageResponse | null // null until the first success, and after a failure
  loading: boolean
  error: string | null // request failed, non-2xx, or unparsable body
}

export function useUsage(usageWindow: UsageWindow): UsageState
```

Required behavior:

1. `GET /api/v1/usage?window=<window>` on mount and on every window change,
   aborted from the cleanup via `AbortController`.
2. Parse through `parseUsage` (see type-safety.md). `null` sets the error state —
   the caller must not render half a chart.
3. **No SSE and no polling.** Aggregates are a snapshot of a window; a stat page
   that quietly rewrites its own numbers is worse than a stale one.
4. Keep the previous payload on screen while the next window loads, but drop it
   on failure: showing the old window's numbers under a newly selected window
   would be a lie.
5. No `enabled` flag. The hook is mounted with the usage tab, and that
   conditional render is what controls when the request happens.

---

## Common Mistakes

- Opening the EventSource in more than one component — exactly one instance,
  owned by `App` via this hook. It must also be called unconditionally, above
  the tab switch: gating it on the active tab reconnects on every switch.
- Putting `devices` in a dependency array of the effect that owns the
  EventSource (causes reconnect loops) — use functional state updates instead.
- Forgetting that SSE auto-reconnect does not replay missed events — the
  snapshot re-fetch on reconnect is mandatory, not an optimization.
