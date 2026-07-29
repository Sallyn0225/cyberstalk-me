# Directory Structure

> How frontend code is organized in this project.

---

## Overview

The frontend is a single-page **React + Vite + TypeScript** app in `web/`,
using **shadcn/ui + Tailwind CSS** for components and styling. It is one public
page (a device card grid) with no routing, no auth, and no forms — keep the
tree flat and resist adding structure the app doesn't need. This layout was
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
    ├── App.tsx              # page shell: header + DeviceGrid (named export)
    ├── index.css            # Tailwind v4 entry + font + shadcn theme tokens
    ├── components/
    │   ├── ui/              # shadcn/ui generated primitives (card.tsx, badge.tsx, ...)
    │   ├── DeviceGrid.tsx   # app components stay flat, one per file, PascalCase
    │   ├── DeviceCard.tsx
    │   └── BatteryIndicator.tsx
    ├── hooks/               # custom hooks, camelCase use*.ts
    │   └── useDeviceStream.ts
    ├── types/
    │   ├── contract.ts      # TS mirror of shared/ Go structs — single source
    │   └── contract.test.ts # tests sit next to the code they cover
    └── lib/                 # pure helpers, no React imports
        ├── utils.ts         # cn() — created by shadcn init
        ├── format.ts        # formatting / time-ago
        └── format.test.ts
```

There is no `web/dist/`: `vite build` writes into `server/cmd/server/web/`, the
directory the Go binary embeds (see quality-guidelines.md).

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
