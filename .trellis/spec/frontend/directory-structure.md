# Directory Structure

> How frontend code is organized in this project.

---

## Overview

The frontend is a single-page **React + Vite + TypeScript** app in `web/`. It is
one public page (a device card grid) with no routing, no auth, and no forms —
keep the tree flat and resist adding structure the app doesn't need. This layout
was seeded from the project design (`.trellis/tasks/07-28-cyberstalk-me/design.md`);
update it if reality diverges.

---

## Directory Layout

```
web/
├── index.html
├── vite.config.ts           # dev proxy: /api -> http://localhost:<server port>
├── tsconfig.json            # strict: true
└── src/
    ├── main.tsx             # ReactDOM entry
    ├── App.tsx              # page shell: header + DeviceGrid
    ├── components/          # one component per file, PascalCase
    │   ├── DeviceGrid.tsx
    │   ├── DeviceCard.tsx
    │   ├── BatteryIndicator.tsx
    │   └── ...
    ├── hooks/               # custom hooks, camelCase use*.ts
    │   └── useDeviceStream.ts
    ├── types/
    │   └── contract.ts      # TS mirror of shared/ Go structs — single source
    ├── lib/                 # pure helpers (formatting, time-ago), no React imports
    │   └── format.ts
    └── styles/              # plain CSS (see component-guidelines.md)
```

---

## Module Organization

- **No feature folders** — the app is one feature. Components go flat in
  `components/`; if a component needs private subcomponents, keep them in the
  same file until they're reused elsewhere.
- `lib/` is for pure functions only (no React, no fetch). Anything that touches
  React state belongs in `hooks/`.
- All contract types live in `types/contract.ts`; nothing else declares API
  payload shapes.

---

## Naming Conventions

- Components: `PascalCase.tsx`, named export matching the filename.
- Hooks: `useXxx.ts`, named export.
- Non-component files: `camelCase.ts`.
- No `index.ts` barrel files — import directly from the file.

---

## Examples

Once the first components land, `DeviceCard.tsx` + `useDeviceStream.ts` are the
reference implementations for new code.
