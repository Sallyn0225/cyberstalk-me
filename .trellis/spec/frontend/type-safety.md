# Type Safety

> Type safety patterns in this project.

---

## Baseline

- TypeScript with `"strict": true`. It lives in `tsconfig.app.json` (the
  project-reference config that actually covers `src/`), not the solution-level
  `tsconfig.json` — the Vite template does not set it, so it must be added by
  hand. Strict stays on; code that needs loosening is wrong, not the config.
- No `baseUrl`: TypeScript 6 deprecates it. `paths` alone resolves `@/*`
  relative to the config file, and `vite.config.ts` mirrors it in
  `resolve.alias`.

---

## Type Organization

- **`src/types/contract.ts` is the single mirror of the Go contract** in the
  `shared/` Go module. Every field name matches the Go struct's JSON tag
  exactly (`snake_case`, e.g. `device_id`, `reported_at`). Nothing else in the
  frontend declares API shapes.
- When the Go contract changes, `contract.ts` changes in the same task — this
  is a hard cross-layer rule (see backend quality checklist).
- Nullability follows the Go type, not convenience: a **Go pointer field** is
  `| null` here, a **Go value field** is not nullable. Use `| null`, never
  optional `?` — the backend serializes explicit `null`, and `| null` forces
  handling at use sites.
  - `Activity` is a Go value type in `DeviceState` → `activity: Activity`.
  - `Battery.Level` is `*int` → `level: number | null`.
  - `*Battery` / `*NetworkType` → `battery: Battery | null`, `network: … | null`.

```ts
// types/contract.ts — mirrors shared/contract.go
export type DeviceType = 'windows' | 'android'
export type NetworkType = 'wifi' | 'cellular' | 'ethernet' | 'offline'

export interface Activity {
  app: string
  description: string
  idle: boolean
  idle_seconds: number
}

export interface Battery {
  level: number | null // 0-100; Go *int, null when the OS can't report it
  charging: boolean
}

export interface DeviceState {
  device_id: string
  device_name: string
  device_type: DeviceType
  activity: Activity // Go value type — never null
  battery: Battery | null
  network: NetworkType | null
  online: boolean
  reported_at: string // RFC 3339
  last_seen_at: string // RFC 3339
}

export type StreamEvent =
  | { type: 'update'; device: DeviceState }
  | { type: 'offline'; device: DeviceState }
```

---

## Validation

- **No Zod/Yup.** The backend is ours and the contract is narrow; runtime
  validation is a hand-written type guard at the single trust boundary (SSE
  message / snapshot parse).
- Guards check **structure only, never the string-union values**. The backend
  validates `device_type` on report but does not constrain `network` at all,
  and a kind added later must degrade to a generic label in the UI rather than
  make the whole device vanish. Label lookups therefore need a fallback
  (`LABELS[value] ?? value`).
- `GET /api/v1/snapshot` returns a **bare array**, not an envelope — parse it
  as `DeviceState[]`, and reject the payload if any entry is malformed.

```ts
export function parseStreamEvent(raw: string): StreamEvent | null {
  let data: unknown
  try {
    data = JSON.parse(raw)
  } catch {
    return null
  }
  if (!isRecord(data)) return null
  if (data.type !== 'update' && data.type !== 'offline') return null
  if (!isDeviceState(data.device)) return null
  return data as StreamEvent
}
```

Returning `null` (caller warns and drops) keeps a bad event from crashing the
page.

---

## Forbidden Patterns

- `any` — use `unknown` and narrow. (`as` casts only inside the parse helpers
  above, after the guard checks.)
- `// @ts-ignore` / `@ts-expect-error` in app code.
- Declaring API payload types anywhere outside `types/contract.ts`.
- `enum` — use string literal unions (as above); they match JSON wire values
  directly.
