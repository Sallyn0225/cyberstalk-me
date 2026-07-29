# Quality Guidelines

> Code quality standards for frontend development.

---

## Tooling (must pass before any commit)

```bash
npm run lint        # oxlint (react + typescript + oxc + jsx-a11y plugins)
npm run typecheck   # tsc -b (Vite dev server does not type-check)
npx vitest run      # unit tests for pure logic
npm run build       # tsc -b && vite build
```

- **Linter is oxlint**, not ESLint — that is what `npm create vite` scaffolds
  now, and it implements the React hooks rules this project cares about.
  `.oxlintrc.json` raises `react/exhaustive-deps` from its default warning to
  **error** (rule output is labelled `react-hooks(exhaustive-deps)`); silence it
  only with a written reason. `react/only-export-components` is disabled via an
  `overrides` entry for `src/components/ui/**`, because shadcn-generated files
  legitimately export variant objects alongside components.
- Formatting: Prettier defaults, run via editor; no custom config debates.

---

## Build output is part of the commit

`vite build` writes straight into `server/cmd/server/web/`, the directory the
Go server embeds (`//go:embed all:web`). That keeps deployment to "copy one
binary", at the price of one rule:

> **Any frontend change must be `npm run build`-ed and committed together with
> its build output.** Otherwise the binary silently serves the previous UI.

`emptyOutDir: true` is mandatory in `vite.config.ts` because the output
directory lives outside the Vite root.

---

## Required Patterns

- All contract data flows through `types/contract.ts` types (see
  type-safety.md).
- Effects that create resources (EventSource, intervals) must return a cleanup
  function.
- Nullable fields (`battery`, `activity`, `network`) rendered with explicit
  null branches — the UI must look intentional for a desktop PC with no
  battery, not broken.
- Offline devices remain visible with distinct styling (product requirement
  AC3) — never filter them out.
- UI primitives come from shadcn/ui, icons from `lucide-react`, animation from
  Framer Motion — hand-write none of these (see component-guidelines.md
  "Component Sourcing").

---

## Forbidden Patterns

- Adding runtime dependencies without recording the reason. Approved baseline:
  `react`, `react-dom`, the shadcn/ui stack (`tailwindcss`, `@tailwindcss/vite`,
  `radix-ui`, `class-variance-authority`, `clsx`, `tailwind-merge`,
  `tw-animate-css`), `lucide-react` for icons, Framer Motion (`motion`) for
  animation, and `@fontsource-variable/plus-jakarta-sans` — the theme's font,
  self-hosted so the public site makes no third-party request and the embedded
  binary works offline. Anything beyond that needs a note in the task PRD.
  (No axios — use `fetch`; no moment/dayjs — `Intl` and a small `timeAgo`
  helper cover the MVP.)
- `dangerouslySetInnerHTML` — all displayed strings come from device reports;
  even sanitized, they render as text only.
- `useEffect` for derived computation — derive in render.
- Class components, PropTypes, default exports.

---

## Testing Requirements

- **Vitest** for pure logic: `lib/format.ts` (time-ago edge cases) and
  `types/contract.ts` parse guards (malformed event → `null`) must have tests.
- Component/E2E tests are not required for the MVP; verification is manual
  against the running backend (see task acceptance criteria AC1–AC5).

---

## Code Review Checklist

- [ ] `npm run lint`, `npm run typecheck`, `npx vitest run`, `npm run build`
      all clean
- [ ] Build output under `server/cmd/server/web/` regenerated and staged
- [ ] No new dependency without a recorded reason
- [ ] Contract mirror in sync with `shared/` Go structs
- [ ] EventSource/interval cleanup present in every effect that creates one
- [ ] Null branches rendered intentionally; offline state visually distinct
- [ ] No hand-rolled UI primitive, inline SVG icon, or CSS keyframe animation
      where shadcn/ui, lucide-react, or Framer Motion covers it
