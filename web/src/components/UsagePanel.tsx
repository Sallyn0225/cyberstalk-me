import { useState } from 'react'
import { CircleAlert, Monitor, Smartphone, type LucideIcon } from 'lucide-react'

import { UsageAppList } from '@/components/UsageAppList'
import { UsageChart, type UsageChartSlot } from '@/components/UsageChart'
import { UsageTotals } from '@/components/UsageTotals'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useUsage } from '@/hooks/useUsage'
import { deviceTypeLabel, formatDay, formatHour } from '@/lib/format'
import type { DeviceUsage, UsageWindow } from '@/types/contract'

const DEVICE_ICONS: Record<string, LucideIcon> = {
  windows: Monitor,
  android: Smartphone,
}

const WINDOWS: { value: UsageWindow; label: string }[] = [
  { value: 'today', label: '今日' },
  { value: '7d', label: '近 7 天' },
  { value: '30d', label: '近 30 天' },
]

/** Label every Nth day so a 30-day axis stays legible. */
const DAY_TICK_STRIDE = 5

/**
 * Maps whichever series the server filled into the chart's slot shape. `hourly`
 * wins when both are set, which cannot happen today but keeps the choice
 * explicit instead of depending on the requested window.
 */
function chartSlots(device: DeviceUsage): UsageChartSlot[] {
  if (device.hourly !== null) {
    return device.hourly.map((slot) => ({
      key: String(slot.hour),
      label: formatHour(slot.hour),
      seconds: slot.seconds,
      topApp: slot.top_app,
      tick: slot.hour % 6 === 0,
    }))
  }
  const daily = device.daily ?? []
  return daily.map((slot, index) => ({
    key: slot.date,
    label: formatDay(slot.date),
    seconds: slot.seconds,
    topApp: slot.top_app,
    tick:
      daily.length <= 7 ||
      index % DAY_TICK_STRIDE === 0 ||
      index === daily.length - 1,
  }))
}

function PanelSkeleton() {
  return (
    <Card>
      <CardContent className="flex flex-col gap-6">
        <Skeleton className="h-2 w-full" />
        <div className="grid gap-3 sm:grid-cols-3">
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
        </div>
        <Skeleton className="h-28 w-full sm:h-36" />
      </CardContent>
    </Card>
  )
}

function PanelNotice({ children }: { children: React.ReactNode }) {
  return (
    <Card>
      <CardContent className="text-muted-foreground flex items-center gap-2">
        {children}
      </CardContent>
    </Card>
  )
}

/**
 * The usage tab: window and device selection on top, then the selected device's
 * totals, app ranking and distribution.
 *
 * Owns the two pieces of local UI state (window, device) and the usage request.
 * The window lives here rather than in `App` because nothing above this tab
 * cares about it, and `useUsage` is mounted with the tab so the statistics
 * endpoint is never hit on a plain page load.
 */
export function UsagePanel() {
  const [usageWindow, setUsageWindow] = useState<UsageWindow>('today')
  const [deviceId, setDeviceId] = useState<string | null>(null)
  const { data, loading, error } = useUsage(usageWindow)

  const devices = data?.devices ?? []
  // Derived at render: an id that no longer exists (or was never picked) falls
  // back to the first device instead of being repaired in an effect.
  const device = devices.find((d) => d.device_id === deviceId) ?? devices.at(0) ?? null

  function selectWindow(value: string) {
    const next = WINDOWS.find((w) => w.value === value)
    // Radix reports "" when the active item is clicked again; a window must
    // always be selected, so an empty value is ignored.
    if (next !== undefined) setUsageWindow(next.value)
  }

  const DeviceIcon =
    device === null ? Monitor : (DEVICE_ICONS[device.device_type] ?? Monitor)
  const totalSeconds =
    device === null
      ? 0
      : Math.max(device.totals.active_seconds, 0) +
        Math.max(device.totals.idle_seconds, 0) +
        Math.max(device.totals.locked_seconds, 0)
  const slots = device === null ? [] : chartSlots(device)
  const windowLabel = WINDOWS.find((w) => w.value === data?.window)?.label ?? '这段时间'

  return (
    <section className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-3">
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          spacing={0}
          value={usageWindow}
          onValueChange={selectWindow}
          aria-label="统计窗口"
        >
          {WINDOWS.map((w) => (
            <ToggleGroupItem key={w.value} value={w.value}>
              {w.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>

        {devices.length > 1 ? (
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            spacing={0}
            value={device?.device_id ?? ''}
            onValueChange={(value) => {
              if (value !== '') setDeviceId(value)
            }}
            aria-label="设备"
          >
            {devices.map((d) => (
              <ToggleGroupItem key={d.device_id} value={d.device_id}>
                {d.device_name}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        ) : null}

        {loading && data !== null ? (
          <span className="text-muted-foreground text-xs">更新中</span>
        ) : null}
      </div>

      {loading && data === null ? (
        <PanelSkeleton />
      ) : error !== null || data === null ? (
        <PanelNotice>
          <CircleAlert aria-hidden className="text-destructive size-4 shrink-0" />
          {error ?? '拉取使用时间统计失败'}
        </PanelNotice>
      ) : device === null ? (
        <PanelNotice>
          还没有设备上报过。启动任意一台设备上的采集客户端，它就会出现在这里。
        </PanelNotice>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="flex min-w-0 items-center gap-2">
              <DeviceIcon aria-hidden className="text-muted-foreground size-4 shrink-0" />
              <h2 className="truncate">{device.device_name}</h2>
            </CardTitle>
            <CardDescription>
              {deviceTypeLabel(device.device_type)} · {windowLabel}
            </CardDescription>
          </CardHeader>

          {totalSeconds === 0 ? (
            <CardContent className="text-muted-foreground">
              {windowLabel}内这台设备没有任何记录。功能上线前的时间没有统计数据，长窗口在初期只会有一部分。
            </CardContent>
          ) : (
            <CardContent className="flex flex-col gap-8">
              <UsageTotals totals={device.totals} />

              <div className="flex flex-col gap-3">
                <h3 className="text-sm font-medium">应用排行</h3>
                <UsageAppList apps={device.apps} />
              </div>

              {slots.length > 0 ? (
                <div className="flex flex-col gap-3">
                  <h3 className="text-sm font-medium">活跃分布</h3>
                  <UsageChart
                    slots={slots}
                    caption={
                      device.hourly !== null
                        ? `按 ${data.timezone} 的本地小时划分`
                        : `按 ${data.timezone} 的本地日期划分`
                    }
                  />
                </div>
              ) : null}
            </CardContent>
          )}
        </Card>
      )}
    </section>
  )
}
