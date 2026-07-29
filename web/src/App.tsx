import { useEffect, useState } from 'react'
import { CircleAlert, Eye, Radio } from 'lucide-react'

import { DeviceGrid } from '@/components/DeviceGrid'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useDeviceStream, type ConnectionState } from '@/hooks/useDeviceStream'
import { cn } from '@/lib/utils'

/** How often the page re-renders so "最后活跃 X 前" stays honest. */
const RELATIVE_TIME_REFRESH_MS = 30_000

const CONNECTION_LABELS: Record<ConnectionState, string> = {
  connecting: '连接中',
  live: '实时',
  reconnecting: '重连中',
}

function ConnectionStatus({ connection }: { connection: ConnectionState }) {
  return (
    <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
      <Radio
        aria-hidden
        className={cn('size-3.5', connection === 'live' && 'text-primary')}
      />
      {CONNECTION_LABELS[connection]}
    </span>
  )
}

function LoadingGrid() {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {[0, 1, 2].map((i) => (
        <Card key={i} className="h-40">
          <CardContent className="flex flex-col gap-3">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-6 w-40" />
            <Skeleton className="h-4 w-24" />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

export function App() {
  const { devices, connection, error } = useDeviceStream()

  // Relative times are derived at render time, so a periodic re-render is all
  // that keeps them fresh — nothing formatted is stored in state.
  const [, setTick] = useState(0)
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), RELATIVE_TIME_REFRESH_MS)
    return () => clearInterval(id)
  }, [])

  const empty = devices.length === 0

  return (
    <main className="mx-auto flex min-h-dvh max-w-5xl flex-col gap-8 px-4 py-10 sm:px-6">
      <header className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="font-heading flex items-center gap-2 text-2xl font-semibold">
            <Eye aria-hidden className="text-primary size-6" />
            赛博视奸
          </h1>
          <ConnectionStatus connection={connection} />
        </div>
        <p className="text-muted-foreground text-sm">
          这里实时显示我此刻在各设备上干什么。看着就好，别打扰。
        </p>
      </header>

      {empty && error ? (
        <Card>
          <CardContent className="text-muted-foreground flex items-center gap-2">
            <CircleAlert aria-hidden className="text-destructive size-4" />
            {error}
          </CardContent>
        </Card>
      ) : empty && connection === 'connecting' ? (
        <LoadingGrid />
      ) : empty ? (
        <Card>
          <CardContent className="text-muted-foreground">
            还没有设备上报过。启动任意一台设备上的采集客户端，它就会出现在这里。
          </CardContent>
        </Card>
      ) : (
        <DeviceGrid devices={devices} />
      )}
    </main>
  )
}
