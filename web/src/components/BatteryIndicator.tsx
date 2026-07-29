import { Battery as BatteryIcon, BatteryCharging } from 'lucide-react'

import { cn } from '@/lib/utils'
import type { Battery } from '@/types/contract'

interface BatteryIndicatorProps {
  battery: Battery | null
}

/**
 * Power state. Renders nothing for machines with no battery at all (Go sends
 * `battery: null` for desktops), and drops the gauge when the OS reports a
 * battery but no percentage.
 */
export function BatteryIndicator({ battery }: BatteryIndicatorProps) {
  if (battery === null) return null

  const { level, charging } = battery
  const Icon = charging ? BatteryCharging : BatteryIcon

  if (level === null) {
    return (
      <span className="flex items-center gap-1.5">
        <Icon aria-hidden className="size-3.5" />
        <span>{charging ? '充电中' : '电池'}</span>
      </span>
    )
  }

  const percent = Math.min(100, Math.max(0, Math.round(level)))

  return (
    <span className="flex items-center gap-1.5">
      <Icon aria-hidden className="size-3.5" />
      <span className="bg-muted h-1.5 w-10 overflow-hidden rounded-full">
        <span
          className={cn(
            'block h-full rounded-full',
            percent <= 20 ? 'bg-destructive' : 'bg-primary',
          )}
          // The only legitimate inline style here: a genuinely dynamic value.
          style={{ width: `${percent}%` }}
        />
      </span>
      <span className="tabular-nums">{percent}%</span>
      <span className="sr-only">{charging ? '充电中' : '未充电'}</span>
    </span>
  )
}
