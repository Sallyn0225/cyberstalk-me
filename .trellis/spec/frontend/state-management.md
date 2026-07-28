# State Management

> How state is managed in this project.

---

## Overview

**No state library.** No Redux, Zustand, Jotai, or Context providers. The app's
entire state fits in one hook (`useDeviceStream`) owned by `App` and passed down
as props. This is a deliberate decision — revisit only if the app grows real
client-side features (filters, settings), and record the change here.

---

## State Categories

| Category | Where it lives | Example |
|----------|----------------|---------|
| Server state | `useDeviceStream` in `App` | device list, per-device status |
| Connection meta | same hook | `"connecting" / "live" / "reconnecting"` banner |
| Local UI state | `useState` inside the owning component | a card's expanded tooltip |
| Derived state | computed in render, not stored | offline flag from `online`, sorted card order |

- Props flow one level: `App` → `DeviceGrid` → `DeviceCard`. At this depth,
  prop drilling is correct; do not introduce Context to avoid two levels of
  props.

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

---

## Common Mistakes

- Storing derived data (e.g. `isOffline`, formatted times) in state — derive in
  render; the server's `online` field plus timestamps are sufficient.
- Duplicating the device list into a second piece of state for the connection
  banner — the hook already returns both.
