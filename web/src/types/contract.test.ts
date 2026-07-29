import { describe, expect, it } from 'vitest'

import {
  isDeviceState,
  parseSnapshot,
  parseStreamEvent,
  type DeviceState,
} from '@/types/contract'

/** A well-formed device, mirroring what the Go server encodes. */
function device(overrides: Partial<DeviceState> = {}): DeviceState {
  return {
    device_id: 'win-desktop',
    device_name: '我的台式机',
    device_type: 'windows',
    activity: {
      app: 'VS Code',
      description: '在写代码',
      idle: false,
      idle_seconds: 0,
    },
    battery: { level: 82, charging: true },
    network: 'wifi',
    online: true,
    reported_at: '2026-07-29T12:00:00Z',
    last_seen_at: '2026-07-29T12:00:01Z',
    ...overrides,
  }
}

describe('isDeviceState', () => {
  it('accepts the nullable shapes the Go contract really produces', () => {
    // Desktop with no battery and no reported network.
    expect(isDeviceState(device({ battery: null, network: null }))).toBe(true)
    // Battery present but the OS cannot report a percentage (Go *int).
    expect(isDeviceState(device({ battery: { level: null, charging: false } }))).toBe(
      true,
    )
  })

  it('accepts device and network kinds it does not know yet', () => {
    // Structure is validated, string unions deliberately are not — a new
    // backend kind must degrade in the UI, not vanish from it.
    expect(isDeviceState(device({ device_type: 'linux' as never }))).toBe(true)
    expect(isDeviceState(device({ network: 'bluetooth' as never }))).toBe(true)
  })

  it('rejects missing or wrongly typed required fields', () => {
    expect(isDeviceState(null)).toBe(false)
    expect(isDeviceState([device()])).toBe(false)
    expect(isDeviceState({ ...device(), device_id: 42 })).toBe(false)
    expect(isDeviceState({ ...device(), online: 'yes' })).toBe(false)
    // activity is a Go value type: null is not a legal payload.
    expect(isDeviceState({ ...device(), activity: null })).toBe(false)
    expect(isDeviceState({ ...device(), activity: { app: 'VS Code' } })).toBe(false)
    expect(
      isDeviceState({ ...device(), battery: { level: 82 } }),
    ).toBe(false)
  })
})

describe('parseSnapshot', () => {
  it('accepts a bare array — the endpoint has no envelope', () => {
    expect(parseSnapshot([])).toEqual([])
    expect(parseSnapshot([device(), device({ device_id: 'phone' })])).toHaveLength(2)
  })

  it('rejects an envelope object or any other non-array', () => {
    expect(parseSnapshot({ devices: [device()] })).toBeNull()
    expect(parseSnapshot(null)).toBeNull()
    expect(parseSnapshot('[]')).toBeNull()
  })

  it('rejects the whole payload when any entry is malformed', () => {
    expect(parseSnapshot([device(), { device_id: 'broken' }])).toBeNull()
  })
})

describe('parseStreamEvent', () => {
  it('parses update and offline events', () => {
    const update = parseStreamEvent(
      JSON.stringify({ type: 'update', device: device() }),
    )
    expect(update?.type).toBe('update')
    expect(update?.device.device_id).toBe('win-desktop')

    const offline = parseStreamEvent(
      JSON.stringify({ type: 'offline', device: device({ online: false }) }),
    )
    expect(offline?.type).toBe('offline')
    expect(offline?.device.online).toBe(false)
  })

  it('drops events with an unknown type', () => {
    expect(
      parseStreamEvent(JSON.stringify({ type: 'deleted', device: device() })),
    ).toBeNull()
  })

  it('drops events with a malformed or missing device', () => {
    expect(parseStreamEvent(JSON.stringify({ type: 'update' }))).toBeNull()
    expect(
      parseStreamEvent(JSON.stringify({ type: 'update', device: { device_id: 1 } })),
    ).toBeNull()
  })

  it('drops non-JSON payloads instead of throwing', () => {
    expect(parseStreamEvent('')).toBeNull()
    expect(parseStreamEvent('{')).toBeNull()
    // The server opens the stream with `event: ready` / `data: {}`; that frame
    // never reaches onmessage, but an empty object must be harmless anyway.
    expect(parseStreamEvent('{}')).toBeNull()
  })
})
