# Quality Guidelines

> Code quality standards for frontend development.

---

## Tooling (must pass before any commit)

```bash
npm run lint        # ESLint flat config: @eslint/js + typescript-eslint + react-hooks
npx tsc --noEmit    # type-check (Vite dev server does not type-check)
npm run build       # vite build must succeed
```

- ESLint with the standard Vite React-TS template setup (`typescript-eslint`
  recommended + `eslint-plugin-react-hooks`). The `react-hooks/exhaustive-deps`
  rule is an error, not a warning — silence it only with a written reason.
- Formatting: Prettier defaults, run via editor; no custom config debates.

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

---

## Forbidden Patterns

- Adding runtime dependencies without recording the reason. Approved baseline:
  `react`, `react-dom` only. Icon or time-formatting libraries need a note in
  the task PRD. (No axios — use `fetch`; no moment/dayjs — `Intl` and a small
  `timeAgo` helper cover the MVP.)
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

- [ ] `tsc --noEmit`, `npm run lint`, `npm run build` all clean
- [ ] No new dependency without a recorded reason
- [ ] Contract mirror in sync with `shared/` Go structs
- [ ] EventSource/interval cleanup present in every effect that creates one
- [ ] Null branches rendered intentionally; offline state visually distinct
