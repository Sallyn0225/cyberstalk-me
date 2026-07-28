# Component Guidelines

> How components are built in this project.

---

## Component Structure

- Function components only, defined with `function` declarations and a named
  export:

```tsx
// components/DeviceCard.tsx
import type { DeviceState } from "../types/contract";
import { timeAgo } from "../lib/format";

interface DeviceCardProps {
  device: DeviceState;
}

export function DeviceCard({ device }: DeviceCardProps) {
  const offline = !device.online;
  return (
    <article className={`device-card${offline ? " device-card--offline" : ""}`}>
      <h2>{device.device_name}</h2>
      {/* ... */}
    </article>
  );
}
```

- Components are **presentational**: they receive device data via props and
  render it. All data acquisition lives in `useDeviceStream` at the `App`
  level — components below `App` never fetch or subscribe.
- Keep components small; extract a child component when a JSX block needs its
  own props, not before.

---

## Props Conventions

- Props interface named `<Component>Props`, declared in the same file, not
  exported unless another file genuinely needs it.
- Destructure props in the signature.
- No `React.FC` — annotate props directly (as above).
- Optional/nullable contract fields (e.g. `battery` may be `null` for desktop
  PCs) are handled by the component with graceful degradation: render nothing
  or a placeholder, never crash and never invent a value.

---

## Styling Patterns

- **Plain CSS** with BEM-ish class names (`device-card`, `device-card--offline`),
  in `src/styles/`. No Tailwind, no CSS-in-JS, no CSS modules — the app is one
  page and plain CSS keeps the toolchain minimal.
- State variants are modifier classes toggled from props (see example above),
  not inline styles. Inline styles only for truly dynamic values (battery bar
  width).

---

## Accessibility

- Card grid is semantic: `<main>` → `<article>` per device, device name in a
  heading.
- Online/offline must not be conveyed by color alone — pair the indicator with
  text ("在线" / "离线").
- Icons get `aria-label` or accompanying visible text.

---

## Common Mistakes

- Fetching or opening an `EventSource` inside `DeviceCard` — subscription is
  centralized in one hook.
- Formatting timestamps inline in JSX — use `lib/format.ts` helpers so
  "last seen" wording stays consistent across components.
