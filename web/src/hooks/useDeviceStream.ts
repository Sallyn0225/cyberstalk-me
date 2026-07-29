import { useEffect, useState } from 'react'

import { parseSnapshot, parseStreamEvent, type DeviceState } from '@/types/contract'

const SNAPSHOT_URL = '/api/v1/snapshot'
const STREAM_URL = '/api/v1/stream'

export type ConnectionState = 'connecting' | 'live' | 'reconnecting'

export interface DeviceStreamState {
  /** Stable order by device_id so cards never jump when an event arrives. */
  devices: DeviceState[]
  connection: ConnectionState
  /** Set when the initial snapshot fails; SSE drops are not errors. */
  error: string | null
}

function byDeviceId(a: DeviceState, b: DeviceState): number {
  return a.device_id.localeCompare(b.device_id)
}

/** Replaces the device with the same id, or inserts it in sorted position. */
function upsert(devices: DeviceState[], next: DeviceState): DeviceState[] {
  const index = devices.findIndex((d) => d.device_id === next.device_id)
  if (index === -1) return [...devices, next].sort(byDeviceId)
  const merged = devices.slice()
  merged[index] = next
  return merged
}

/**
 * The app's only server-state source: one snapshot fetch plus one SSE
 * subscription, owned by `App`. No component below it fetches or subscribes.
 */
export function useDeviceStream(): DeviceStreamState {
  const [devices, setDevices] = useState<DeviceState[]>([])
  const [connection, setConnection] = useState<ConnectionState>('connecting')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    let cancelled = false
    // Flipped on the first successful open. Any open after that is a
    // *re*connect, which must re-sync: EventSource retries transparently but
    // never replays the events missed while it was down.
    let connectedOnce = false

    async function loadSnapshot(): Promise<void> {
      try {
        const response = await fetch(SNAPSHOT_URL, { signal: controller.signal })
        if (!response.ok) throw new Error(`snapshot responded ${response.status}`)
        const parsed = parseSnapshot(await response.json())
        if (cancelled) return
        if (parsed === null) {
          console.warn('discarded malformed snapshot payload')
          return
        }
        setDevices(parsed.slice().sort(byDeviceId))
        setError(null)
      } catch (err) {
        if (cancelled || controller.signal.aborted) return
        console.warn('snapshot fetch failed', err)
        setError('拉取设备快照失败')
      }
    }

    void loadSnapshot()

    const source = new EventSource(STREAM_URL)

    source.onopen = () => {
      setConnection('live')
      setError(null)
      if (connectedOnce) void loadSnapshot()
      connectedOnce = true
    }

    source.onmessage = (event) => {
      const parsed = parseStreamEvent(event.data)
      if (parsed === null) {
        console.warn('dropped malformed stream event', event.data)
        return
      }
      // Functional update: the effect must not depend on `devices`, or every
      // event would tear down and rebuild the EventSource.
      setDevices((prev) => upsert(prev, parsed.device))
    }

    source.onerror = () => {
      // EventSource reconnects on its own — recreating it here would fight
      // its backoff and leak connections.
      setConnection('reconnecting')
    }

    return () => {
      cancelled = true
      controller.abort()
      source.close()
    }
  }, [])

  return { devices, connection, error }
}
