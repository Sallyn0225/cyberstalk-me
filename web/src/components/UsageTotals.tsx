import { Activity, Lock, MoonStar, type LucideIcon } from 'lucide-react'

import { formatDuration } from '@/lib/format'
import { sharePercent } from '@/lib/usage'
import { cn } from '@/lib/utils'
import type { UsageTotals as UsageTotalsData } from '@/types/contract'

interface UsageTotalsProps {
  totals: UsageTotalsData
}

interface Slice {
  label: string
  icon: LucideIcon
  seconds: number
  /** Bar fill. One accent, three lightness steps — never colour alone. */
  bar: string
  value: string
}

/**
 * Active / idle / locked time for the selected device and window.
 *
 * The stacked bar is decoration on top of the numbers: it is `aria-hidden`
 * because the same three durations are already read out as text right below it.
 */
export function UsageTotals({ totals }: UsageTotalsProps) {
  const slices: Slice[] = [
    {
      label: '活跃',
      icon: Activity,
      seconds: totals.active_seconds,
      bar: 'bg-primary',
      value: 'text-primary',
    },
    {
      label: '挂机',
      icon: MoonStar,
      seconds: totals.idle_seconds,
      bar: 'bg-primary/40',
      value: 'text-foreground',
    },
    {
      label: '锁屏',
      icon: Lock,
      seconds: totals.locked_seconds,
      bar: 'bg-muted-foreground/40',
      value: 'text-foreground',
    },
  ]

  // Derived at render, never stored. `sharePercent` short-circuits the empty
  // window, where every slice and the total are 0.
  const total = slices.reduce((sum, slice) => sum + Math.max(slice.seconds, 0), 0)

  return (
    <div className="flex flex-col gap-3">
      <div
        aria-hidden
        className="bg-muted/50 flex h-2 w-full overflow-hidden rounded-full"
      >
        {slices.map((slice) => (
          <div
            key={slice.label}
            className={slice.bar}
            style={{ width: `${sharePercent(slice.seconds, total)}%` }}
          />
        ))}
      </div>

      <dl className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-3">
        {slices.map((slice) => (
          <div key={slice.label} className="flex items-baseline gap-2 sm:flex-col sm:gap-1">
            <dt className="text-muted-foreground flex items-center gap-1.5 text-xs">
              <slice.icon aria-hidden className="size-3.5" />
              {slice.label}
            </dt>
            {/* tabular-nums, not font-mono: the durations mix digits with 小时
                / 分, and the mono fallback for CJK spaces them far apart. */}
            <dd className={cn('text-xl font-medium tabular-nums', slice.value)}>
              {formatDuration(slice.seconds)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
