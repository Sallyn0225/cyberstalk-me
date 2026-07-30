# Directory Structure

> How frontend code is organized in this project.

---

## Overview

The frontend is a single-page **React + Vite + TypeScript** app in `web/`,
using **shadcn/ui + Tailwind CSS** for components and styling. It is one public
page with two tabs (live device cards, usage statistics) and **no router** — the
tab is `useState` in `App`, so the server's static fallback stays a single route
and there are no deep links to keep working. No auth, no forms; keep the tree
flat and resist adding structure the app doesn't need. This layout was
seeded from the project design (`.trellis/tasks/07-28-cyberstalk-me/design.md`);
update it if reality diverges.

---

## Directory Layout

```
web/
├── index.html               # lang="zh-CN" class="dark" (dark-only site)
├── components.json          # shadcn/ui config (style radix-nova, aliases)
├── .oxlintrc.json           # linter config; exhaustive-deps raised to error
├── vite.config.ts           # @tailwindcss/vite, "@" alias, dev proxy /api -> :8080,
│                            #   build.outDir -> ../server/cmd/server/web, vitest config
├── tsconfig.json            # solution file; paths: "@/*" -> "./src/*"
├── tsconfig.app.json        # strict: true lives here (covers src/)
├── public/
│   └── favicon.svg
└── src/
    ├── main.tsx             # ReactDOM entry
    ├── App.tsx              # page shell: header + tabs (此刻 / 使用时间), named export
    ├── index.css            # Tailwind v4 entry + font + shadcn theme tokens
    ├── components/
    │   ├── ui/              # shadcn/ui generated primitives (card.tsx, tabs.tsx, ...)
    │   ├── DeviceGrid.tsx   # app components stay flat, one per file, PascalCase
    │   ├── DeviceCard.tsx
    │   ├── BatteryIndicator.tsx
    │   ├── UsagePanel.tsx   # usage tab: window + device selection, owns useUsage
    │   ├── UsageTotals.tsx  # active / idle / locked totals
    │   ├── UsageAppList.tsx # two-level app ranking (Collapsible)
    │   └── UsageChart.tsx   # hourly or daily distribution, one component for both
    ├── hooks/               # custom hooks, camelCase use*.ts
    │   ├── useDeviceStream.ts
    │   └── useUsage.ts
    ├── types/
    │   ├── contract.ts      # TS mirror of shared/ Go structs — single source
    │   └── contract.test.ts # tests sit next to the code they cover
    └── lib/                 # pure helpers, no React imports
        ├── utils.ts         # cn() — created by shadcn init
        ├── format.ts        # formatting / time-ago / durations
        ├── format.test.ts
        ├── usage.ts         # share math for bars (zero-denominator safe)
        └── usage.test.ts
```

There is no `web/dist/`: `vite build` writes into `server/cmd/server/web/`, the
directory the Go binary embeds (see quality-guidelines.md).

---

## The second frontend: `client-windows/webui/`

Since 2026-07-30 there are two Vite apps. `client-windows/webui/` is the
`agent.exe -setup` configuration UI: same stack, same conventions, same
`outDir`-into-the-embedding-directory arrangement (`client-windows/cmd/agent/webui/`),
its own CI job and its own freshness gate.

What is deliberately **not** shared:

- **No shared component library.** The two apps have nothing in common but the
  design language. `web/` renders a public read-only dashboard; `webui/` is a
  local form-heavy tool. Extracting a shared package would couple two things
  that change for unrelated reasons.
- **No web font.** `web/` self-hosts Plus Jakarta Sans; `webui/` uses the system
  stack. It is a local tool that ships inside a 12 MB exe, and its text is
  mostly Chinese, which the Latin face would not cover anyway.
- **Different radius scale.** `web/` uses `--radius: 1.4rem`; `webui/` uses
  `0.65rem`. Pill-shaped inputs read as decoration in a dense configuration
  form. The accent colour *is* shared, so the two still read as one product.
- **Different theme strategy.** `web/` is dark-only (`class="dark"` on `<html>`);
  `webui/` follows `prefers-color-scheme` with `color-scheme: light dark` so the
  browser's own scrollbars and controls match.

Two rules specific to `webui/`:

- **The session token comes from the page, never the URL.** `index.html` carries
  a `__SETUP_TOKEN_PLACEHOLDER__` that the agent replaces at serve time. A URL
  ends up in history and in `Referer`; this token guards an API that can read
  raw window titles.
- **Do not re-implement agent logic in the browser.** Rule matching, regexp
  compilation, escaping and validation are all round trips to the agent
  (`/api/preview`, `/api/regex/test`, `/api/regex/suggest`, `PUT /api/config`).
  JavaScript's regexp engine and Go's RE2 do not agree in the corners, and a
  preview that disagrees with the real thing is worse than no preview.

---

## Module Organization

- **No feature folders** — the app is one feature. App components go flat in
  `components/`; shadcn-generated primitives live under `components/ui/` and
  are added via `npx shadcn@latest add <component>`, not written by hand.
- `lib/` is for pure functions only (no React, no fetch). Anything that touches
  React state belongs in `hooks/`.
- All contract types live in `types/contract.ts`; nothing else declares API
  payload shapes.

---

## Naming Conventions

- App components: `PascalCase.tsx`, named export matching the filename.
- `components/ui/` files keep shadcn's own lowercase naming (`card.tsx`) —
  don't rename generated files.
- Hooks: `useXxx.ts`, named export.
- Non-component files: `camelCase.ts`.
- No `index.ts` barrel files — import directly from the file.

---

## Examples

Once the first components land, `DeviceCard.tsx` + `useDeviceStream.ts` are the
reference implementations for new code.
