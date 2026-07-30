/**
 * Pure usage math. No React, no fetch — see the frontend directory spec.
 *
 * Every bar width and bar height in the usage view is a share of some total,
 * and every one of those totals can legitimately be 0 (an empty window, a
 * device that only ever sat on the lock screen). Keeping the division in one
 * tested place is what keeps `NaN%` and `Infinity%` out of the DOM.
 */

/**
 * `value` as a percentage of `total`, clamped to 0-100.
 *
 * Returns 0 whenever the share is not meaningful: a non-positive or non-finite
 * total, or a non-finite/negative value. Callers can use the result in a
 * `width`/`height` percentage without further guarding.
 */
export function sharePercent(value: number, total: number): number {
  if (!Number.isFinite(value) || !Number.isFinite(total)) return 0
  if (total <= 0 || value <= 0) return 0
  return Math.min(100, (value / total) * 100)
}

/** The largest `seconds` in a slot list, or 0 for an empty one. */
export function maxSeconds(items: readonly { seconds: number }[]): number {
  return items.reduce(
    (max, item) => (Number.isFinite(item.seconds) && item.seconds > max ? item.seconds : max),
    0,
  )
}
