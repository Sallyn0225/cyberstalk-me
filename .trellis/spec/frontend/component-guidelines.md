# Component Guidelines

> How components are built in this project.

---

## Component Sourcing (shadcn/ui first)

- **Don't hand-write UI primitives.** Before writing any component, check
  whether shadcn/ui provides it (`npx shadcn@latest add card badge tooltip
  skeleton ...`). Hand-write only app-specific composition (DeviceCard,
  DeviceGrid) on top of those primitives.
- Generated primitives live in `src/components/ui/` and are owned code — small
  tweaks are fine; needing a rewrite means the wrong primitive was chosen.
- **Icons: `lucide-react` only.** Never hand-draw or inline SVG icons — pick
  the closest lucide icon (`Monitor`, `Smartphone`, `BatteryCharging`, `Wifi`,
  `MoonStar`, ...).
- **Animation: Framer Motion** (`motion` package, `import { motion } from
  "motion/react"`) for enter/exit, layout, and state-change animation. Don't
  hand-write CSS keyframes. Trivial hover/focus transitions use Tailwind
  `transition-*` utilities instead — no library needed there. Tailwind's own
  built-ins (`animate-pulse`) count as utilities, not hand-written keyframes.
- **Theme tokens are generated, not authored.** The palette in `src/index.css`
  comes from a tweakcn theme applied with
  `npx shadcn@latest add <theme-registry-url>`; swapping themes means re-running
  that command, never hand-editing colour values.
- Card composition note: in this shadcn version `CardTitle` renders a `div`.
  Put the real `<h2>` *inside* it so the page keeps its heading structure.

---

## Component Structure

- Function components only, defined with `function` declarations and a named
  export:

```tsx
// components/DeviceCard.tsx
import type { DeviceState } from "@/types/contract";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { timeAgo } from "@/lib/format";

interface DeviceCardProps {
  device: DeviceState;
}

export function DeviceCard({ device }: DeviceCardProps) {
  const offline = !device.online;
  return (
    <Card className={cn("p-4", offline && "opacity-60 grayscale")}>
      <h2 className="text-lg font-semibold">{device.device_name}</h2>
      {/* ... */}
    </Card>
  );
}
```

- Components are **presentational**: they receive their data via props and
  render it. Data acquisition lives in hooks, and a hook is called by the
  component that owns the corresponding piece of UI state: `useDeviceStream` in
  `App` (unconditional, above the tabs), `useUsage` in `UsagePanel` (which owns
  the window selection). Everything below those two — `DeviceCard`,
  `UsageTotals`, `UsageAppList`, `UsageChart` — never fetches or subscribes.
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

- **Tailwind CSS** utilities (installed by `shadcn init`; v4 CSS-first config
  lives in `src/index.css` together with the shadcn theme tokens).
- Conditional variants via `cn()` from `src/lib/utils.ts` (see example above) —
  not string concatenation, not inline styles.
- Respect the shadcn CSS-variable theme: use its semantic classes
  (`bg-card`, `text-muted-foreground`, ...) instead of hardcoding colors the
  theme already names.
- Inline styles only for truly dynamic values (battery bar width percentage).

---

## Charts (no chart library)

- **Bars are `div`s, not a charting dependency.** A ranking bar is a track plus a
  width percentage; a distribution bar is a track plus a height percentage.
  Recharts / Chart.js / D3 are not worth their bundle for this, and the theme's
  own colours are what keeps the charts looking like the rest of the site.
- Percentages go through `sharePercent` in `lib/usage.ts`. Never divide inline:
  every one of these denominators can be 0 (empty window, all-locked device),
  and `NaN%` / `Infinity%` in a `style` attribute is the failure mode.
- The server pads every window to a fixed slot count (24 hours, N days), so the
  frontend never fills gaps itself. Empty slots render as an empty track, which
  is how "no usage at this hour" is shown.
- One accent, lightness steps for the rest (`bg-primary`, `bg-primary/40`,
  `bg-muted-foreground/40`). Do not introduce a second hue to distinguish
  series.
- Mixed CJK/digit strings use `tabular-nums`, not `font-mono`: the mono fallback
  for CJK spaces 小时 / 分 far apart.

---

## Accessibility

- Keep the page semantic: a `<main>` wrapper, one card per device, device name
  in a real heading (`<h2>`).
- Online/offline must not be conveyed by color alone — pair the indicator with
  text ("在线" / "离线").
- **Bar height and bar width are never the only encoding.** Every bar carries a
  text equivalent: an `sr-only` span plus a `title` ("14 时，活跃 32 分，主要应用
  Visual Studio Code"). Decorative bars that merely restate adjacent numbers
  (the totals strip) are `aria-hidden` instead, so nothing is read twice.
- Decorative lucide icons get `aria-hidden`; icons that carry meaning on their
  own get an `aria-label` or adjacent visible text.

---

## Common Mistakes

- Fetching or opening an `EventSource` inside `DeviceCard` — subscription is
  centralized in one hook.
- Formatting timestamps inline in JSX — use `lib/format.ts` helpers so
  "last seen" wording stays consistent across components.
- Hand-rolling a badge/tooltip/skeleton that `npx shadcn add` already provides.
- Inlining SVG paths instead of importing from `lucide-react`.
- Heavily rewriting files in `components/ui/` — choose a better primitive
  instead.
