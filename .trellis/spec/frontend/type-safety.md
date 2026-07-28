# Type Safety

> Type safety patterns in this project.

---

## Baseline

- TypeScript with `"strict": true` in `tsconfig.json`. Strict stays on; code
  that needs loosening is wrong, not the config.

---

## Type Organization

- **`src/types/contract.ts` is the single mirror of the Go contract** in the
  `shared/` Go module. Every field name matches the Go struct's JSON tag
  exactly (`snake_case`, e.g. `device_id`, `reported_at`). Nothing else in the
  frontend declares API shapes.
- When the Go contract changes, `contract.ts` changes in the same task — this
  is a hard cross-layer rule (see backend quality checklist).
- Nullable Go fields (`*Battery`, network unknown) are `| null` in TS, not
  optional `?` — the backend serializes explicit `null`, and `| null` forces
  handling at use sites.

```ts
// types/contract.ts — mirrors shared/contract.go
export type DeviceType = "windows" | "android";
export type NetworkType = "wifi" | "cellular" | "ethernet" | "offline" | null;

export interface Activity {
  app: string;
  description: string;
  idle: boolean;
  idle_seconds: number;
}

export interface Battery {
  level: number;      // 0-100
  charging: boolean;
}

export interface DeviceState {
  device_id: string;
  device_name: string;
  device_type: DeviceType;
  online: boolean;
  activity: Activity | null;
  battery: Battery | null;
  network: NetworkType;
  reported_at: string;   // RFC 3339
  last_seen_at: string;  // RFC 3339
}

export type StreamEvent =
  | { type: "update"; device: DeviceState }
  | { type: "offline"; device: DeviceState };
```

---

## Validation

- **No Zod/Yup.** The backend is ours and the contract is narrow; runtime
  validation is a hand-written type guard at the single trust boundary (SSE
  message / snapshot parse):

```ts
export function parseStreamEvent(raw: string): StreamEvent | null {
  try {
    const data = JSON.parse(raw);
    if (data?.type !== "update" && data?.type !== "offline") return null;
    if (typeof data.device?.device_id !== "string") return null;
    return data as StreamEvent;
  } catch {
    return null;
  }
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
