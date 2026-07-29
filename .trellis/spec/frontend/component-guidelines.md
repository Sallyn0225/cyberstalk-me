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

- **Tailwind CSS** utilities (installed by `shadcn init`; v4 CSS-first config
  lives in `src/index.css` together with the shadcn theme tokens).
- Conditional variants via `cn()` from `src/lib/utils.ts` (see example above) —
  not string concatenation, not inline styles.
- Respect the shadcn CSS-variable theme: use its semantic classes
  (`bg-card`, `text-muted-foreground`, ...) instead of hardcoding colors the
  theme already names.
- Inline styles only for truly dynamic values (battery bar width percentage).

---

## Accessibility

- Keep the page semantic: a `<main>` wrapper, one card per device, device name
  in a real heading (`<h2>`).
- Online/offline must not be conveyed by color alone — pair the indicator with
  text ("在线" / "离线").
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
