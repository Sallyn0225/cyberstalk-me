import {
  Cable,
  MoonStar,
  Monitor,
  Radio,
  Signal,
  Smartphone,
  Wifi,
  WifiOff,
  type LucideIcon,
} from 'lucide-react'

import { BatteryIndicator } from '@/components/BatteryIndicator'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { deviceTypeLabel, formatIdle, networkLabel, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { DeviceState } from '@/types/contract'

const DEVICE_ICONS: Record<string, LucideIcon> = {
  windows: Monitor,
  android: Smartphone,
}

const NETWORK_ICONS: Record<string, LucideIcon> = {
  wifi: Wifi,
  cellular: Signal,
  ethernet: Cable,
  offline: WifiOff,
}

interface DeviceCardProps {
  device: DeviceState
}

/**
 * One device. Purely presentational — every value comes from props, and the
 * offline treatment pairs the grey-out with the literal word "离线" so the
 * state is never carried by colour alone.
 */
export function DeviceCard({ device }: DeviceCardProps) {
  const offline = !device.online
  const DeviceIcon = DEVICE_ICONS[device.device_type] ?? Monitor
  const NetworkIcon =
    device.network === null ? null : (NETWORK_ICONS[device.network] ?? Radio)
  const network = networkLabel(device.network)
  const app = device.activity.app.trim() || '未知应用'
  const description = device.activity.description.trim()

  return (
    <Card
      className={cn(
        'h-full transition-opacity',
        offline && 'opacity-60 grayscale',
      )}
    >
      <CardHeader>
        <CardTitle className="flex min-w-0 items-center gap-2">
          <DeviceIcon aria-hidden className="text-muted-foreground size-4 shrink-0" />
          <h2 className="truncate">{device.device_name}</h2>
        </CardTitle>
        <CardDescription>{deviceTypeLabel(device.device_type)}</CardDescription>
        <CardAction>
          <span className="flex items-center gap-1.5 text-xs">
            <span
              aria-hidden
              className={cn(
                'size-2 rounded-full',
                offline ? 'bg-muted-foreground/50' : 'bg-primary animate-pulse',
              )}
            />
            <span className={cn(!offline && 'text-primary')}>
              {offline ? '离线' : '在线'}
            </span>
          </span>
        </CardAction>
      </CardHeader>

      <CardContent className="flex flex-col gap-2">
        <p className="truncate text-lg font-medium" title={app}>
          {app}
        </p>
        {description ? (
          <p className="text-muted-foreground">{description}</p>
        ) : null}
        {!offline && device.activity.idle ? (
          <Badge variant="secondary" className="w-fit">
            <MoonStar aria-hidden />
            空闲 {formatIdle(device.activity.idle_seconds)}
          </Badge>
        ) : null}
      </CardContent>

      <CardFooter className="text-muted-foreground mt-auto flex-wrap gap-x-4 gap-y-2 text-xs">
        <BatteryIndicator battery={device.battery} />
        {network && NetworkIcon ? (
          <span className="flex items-center gap-1.5">
            <NetworkIcon aria-hidden className="size-3.5" />
            <span>{network}</span>
          </span>
        ) : null}
        <span className="ml-auto">最后活跃 {timeAgo(device.last_seen_at)}</span>
      </CardFooter>
    </Card>
  )
}
