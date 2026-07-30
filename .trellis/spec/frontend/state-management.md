# State Management

> How state is managed in this project.

---

## Overview

**No state library.** No Redux, Zustand, Jotai, or Context providers. Server
state lives in **two independent hooks**, each owned by the component that needs
it, and flows down as props:

| Hook | Owner | Feeds |
|------|-------|-------|
| `useDeviceStream` | `App` (unconditional, above the tabs) | the "此刻" tab: device list + SSE connection state |
| `useUsage(window)` | `UsagePanel` | the "使用时间" tab: aggregated statistics for one window |

Plus a little local UI state: the active tab in `App`, and the selected window
and device inside `UsagePanel`.

That is the whole state story. It is still a deliberate no-library setup —
revisit only if the app grows real client-side features (filters, settings),
and record the change here.

---

## State Categories

| Category | Where it lives | Example |
|----------|----------------|---------|
| Server state | `useDeviceStream` in `App`, `useUsage` in `UsagePanel` | device list, per-device status, usage aggregates |
| Connection meta | `useDeviceStream` | `"connecting" / "live" / "reconnecting"` banner |
| Local UI state | `useState` inside the owning component | active tab, selected window, selected device, an expanded app row |
| Derived state | computed in render, not stored | offline flag from `online`, sorted card order, bar widths, formatted durations |

- Props flow one level: `App` → `DeviceGrid` → `DeviceCard`, and
  `UsagePanel` → `UsageTotals` / `UsageAppList` / `UsageChart`. At this depth,
  prop drilling is correct; do not introduce Context to avoid two levels of
  props.
- **Tabs decide what renders, never whether a hook runs.** `useDeviceStream`
  stays at the top of `App` and is called unconditionally: moving it into the
  "此刻" branch would tear down and rebuild the SSE connection on every tab
  switch. `useUsage` is the opposite case on purpose — it is mounted *with* the
  usage tab, which is what keeps `/api/v1/usage` off a plain page load.

---

## Server State Rules

- The server is the single source of truth. The frontend never mutates device
  state — there are no writes from the browser at all (read-only public site).
- Merge strategy: snapshot replaces the whole list; SSE events upsert one
  device by `device_id`. Keep ordering stable (sort by `device_id`) so cards
  don't jump when events arrive.
- Derived values (time-ago strings, offline styling) are computed at render
  time from the stored state; a 30s interval tick in `App` re-renders to keep
  relative times fresh. Do not store formatted strings in state.
- Usage aggregates are read-only snapshots of a window, **not** a live feed:
  `useUsage` re-fetches when the window changes and nothing else. No SSE, no
  polling. Bar widths, bar heights, sort order and formatted durations are all
  derived in render — see `lib/usage.ts` for the share math, which returns 0
  instead of `NaN` when a window is empty.

---

## Common Mistakes

- Storing derived data (e.g. `isOffline`, formatted times) in state — derive in
  render; the server's `online` field plus timestamps are sufficient.
- Duplicating the device list into a second piece of state for the connection
  banner — the hook already returns both.
- Moving `useDeviceStream` into a tab branch, or gating it behind the active
  tab — that is the SSE-churn bug described above.
- Syncing a selected id into state with an effect. `UsagePanel` resolves the
  selected device in render (`find(id) ?? devices.at(0)`), so a device that
  disappears from the payload needs no repair pass.
