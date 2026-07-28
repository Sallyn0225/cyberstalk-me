# Hook Guidelines

> How hooks are used in this project.

---

## Naming Conventions

- Files and exports: `useXxx` camelCase, one hook per file in `src/hooks/`.
- A hook returns a plain object with named fields, typed explicitly.

---

## Data Fetching

- **No React Query / SWR.** The app has exactly one server-state source:
  snapshot + SSE. A fetching library adds nothing on top of `useDeviceStream`.
- Plain `fetch` for the one-shot snapshot; `EventSource` for the stream.
- API paths are relative (`/api/v1/...`); Vite dev proxy handles local dev.

---

## The canonical hook: `useDeviceStream`

All server communication is encapsulated here. Shape:

```ts
// hooks/useDeviceStream.ts
export interface DeviceStreamState {
  devices: DeviceState[];        // sorted stable by device_id
  connection: "connecting" | "live" | "reconnecting";
}

export function useDeviceStream(): DeviceStreamState
```

Required behavior:

1. On mount: `GET /api/v1/snapshot` → full device list into state.
2. Then open `EventSource("/api/v1/stream")`; each event merges into the list
   keyed by `device_id` (update or insert, never duplicate).
3. **Cleanup on unmount**: `es.close()` in the effect cleanup — leaking
   EventSources is the classic bug here.
4. On `onerror`: set `connection: "reconnecting"` and rely on EventSource's
   built-in retry; re-fetch the snapshot after reconnect to heal missed events.
5. Parse events through the typed helpers in `types/contract.ts`
   (see type-safety.md); malformed events are dropped with a `console.warn`,
   they must not crash the page.

State merging uses `useReducer` when the update logic grows beyond a single
`setDevices` call; start with `useState` and refactor when needed.

---

## Common Mistakes

- Opening the EventSource in more than one component — exactly one instance,
  owned by `App` via this hook.
- Putting `devices` in a dependency array of the effect that owns the
  EventSource (causes reconnect loops) — use functional state updates instead.
- Forgetting that SSE auto-reconnect does not replay missed events — the
  snapshot re-fetch on reconnect is mandatory, not an optimization.
