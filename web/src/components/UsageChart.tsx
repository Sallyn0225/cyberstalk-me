import { formatDuration } from '@/lib/format'
import { maxSeconds, sharePercent } from '@/lib/usage'

/**
 * One bar. `UsagePanel` maps either `hourly` or `daily` into this shape, which
 * is why one chart covers both windows instead of branching on the payload.
 */
export interface UsageChartSlot {
  /** Stable react key: the hour number or the ISO date. */
  key: string
  /** Axis label, e.g. "14 时" or "7/30". */
  label: string
  seconds: number
  /** "" when the slot has no usage. */
  topApp: string
  /** Print the label under this bar. Sparse ticks keep the axis readable. */
  tick: boolean
}

interface UsageChartProps {
  slots: UsageChartSlot[]
  /** Why the slots are cut where they are, e.g. the timezone they follow. */
  caption: string
}

/** Shortest bar that is still visible, so a one-minute slot is not swallowed. */
const MIN_VISIBLE_PERCENT = 4

/**
 * Active-time distribution as plain divs — 24 hour slots or N day slots, all of
 * them present even when empty, because the server pads the window.
 *
 * Height alone carries no information: every bar also has a text equivalent for
 * screen readers plus a `title` for pointer users.
 */
export function UsageChart({ slots, caption }: UsageChartProps) {
  // Bars are relative to the tallest slot; when the whole window is empty the
  // peak is 0 and every bar collapses to its track instead of dividing by zero.
  const peak = maxSeconds(slots)

  return (
    <figure className="flex flex-col gap-2">
      <p className="text-muted-foreground text-xs">
        {peak > 0 ? `峰值 ${formatDuration(peak)}` : '这段时间没有活跃记录'}
      </p>

      <ul className="flex h-28 items-end gap-[2px] sm:h-36">
        {slots.map((slot) => {
          const text =
            slot.seconds > 0
              ? `${slot.label}，活跃 ${formatDuration(slot.seconds)}${
                  slot.topApp ? `，主要应用 ${slot.topApp}` : ''
                }`
              : `${slot.label}，无活跃记录`
          const height =
            slot.seconds > 0
              ? Math.max(sharePercent(slot.seconds, peak), MIN_VISIBLE_PERCENT)
              : 0

          return (
            <li key={slot.key} className="h-full min-w-0 flex-1" title={text}>
              <span className="sr-only">{text}</span>
              <span
                aria-hidden
                className="bg-muted/30 flex h-full w-full items-end overflow-hidden rounded-[3px]"
              >
                <span
                  className="bg-primary w-full rounded-[3px]"
                  style={{ height: `${height}%` }}
                />
              </span>
            </li>
          )
        })}
      </ul>

      <div aria-hidden className="flex gap-[2px]">
        {slots.map((slot) => (
          <span
            key={slot.key}
            className="text-muted-foreground min-w-0 flex-1 text-center text-[10px] whitespace-nowrap"
          >
            {slot.tick ? slot.label : ''}
          </span>
        ))}
      </div>

      <figcaption className="text-muted-foreground text-xs">{caption}</figcaption>
    </figure>
  )
}
