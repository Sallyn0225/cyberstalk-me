import { describe, expect, it } from 'vitest'

import {
  isDeviceState,
  parseSnapshot,
  parseStreamEvent,
  parseUsage,
  type DeviceState,
  type DeviceUsage,
  type UsageResponse,
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

  it('treats activity.locked as optional so older agents keep their card', () => {
    const base = device().activity
    // Present and boolean — what a current agent sends.
    expect(isDeviceState(device({ activity: { ...base, locked: true } }))).toBe(true)
    expect(isDeviceState(device({ activity: { ...base, locked: false } }))).toBe(true)
    // Absent — an agent built before the field existed. Its device must still
    // render; the UI reads a missing flag as "not locked".
    expect(isDeviceState(device({ activity: base }))).toBe(true)
    // Present but the wrong type is a real contract break, not an old client.
    expect(
      isDeviceState(device({ activity: { ...base, locked: 'yes' as never } })),
    ).toBe(false)
    expect(
      isDeviceState(device({ activity: { ...base, locked: null as never } })),
    ).toBe(false)
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

/** A well-formed device usage entry for the "today" window. */
function deviceUsage(overrides: Partial<DeviceUsage> = {}): DeviceUsage {
  return {
    device_id: 'win-desktop',
    device_name: '我的台式机',
    device_type: 'windows',
    totals: { active_seconds: 16_320, idle_seconds: 4200, locked_seconds: 3600 },
    apps: [
      {
        app: 'VS Code',
        seconds: 12_000,
        activities: [
          { description: '在写代码', seconds: 9000 },
          { description: '在看日志', seconds: 3000 },
        ],
      },
    ],
    hourly: [{ hour: 14, seconds: 1800, top_app: 'VS Code' }],
    daily: null,
    ...overrides,
  }
}

function usage(overrides: Partial<UsageResponse> = {}): UsageResponse {
  return {
    window: 'today',
    timezone: 'Asia/Shanghai',
    from: '2026-07-29T16:00:00Z',
    to: '2026-07-30T04:30:00Z',
    devices: [deviceUsage()],
    ...overrides,
  }
}

describe('parseUsage', () => {
  it('accepts the today shape (hourly filled, daily null)', () => {
    const parsed = parseUsage(usage())
    expect(parsed?.devices[0].hourly).toHaveLength(1)
    expect(parsed?.devices[0].daily).toBeNull()
  })

  it('accepts the 7d/30d shape (daily filled, hourly null)', () => {
    const parsed = parseUsage(
      usage({
        window: '7d',
        devices: [
          deviceUsage({
            hourly: null,
            daily: [{ date: '2026-07-30', seconds: 1800, top_app: 'VS Code' }],
          }),
        ],
      }),
    )
    expect(parsed?.devices[0].hourly).toBeNull()
    expect(parsed?.devices[0].daily).toHaveLength(1)
  })

  it('accepts an empty window: no devices, or a device with zero everything', () => {
    expect(parseUsage(usage({ devices: [] }))?.devices).toEqual([])
    const idle = parseUsage(
      usage({
        devices: [
          deviceUsage({
            totals: { active_seconds: 0, idle_seconds: 0, locked_seconds: 0 },
            apps: [],
            hourly: [],
          }),
        ],
      }),
    )
    expect(idle?.devices[0].apps).toEqual([])
  })

  it('accepts a window value it does not know yet', () => {
    // Same reason as device_type: structure is checked, the string union is not.
    expect(parseUsage(usage({ window: '90d' as never }))).not.toBeNull()
  })

  it('rejects a missing or wrongly typed envelope field', () => {
    expect(parseUsage(null)).toBeNull()
    expect(parseUsage([deviceUsage()])).toBeNull()
    const { timezone: _timezone, ...noTimezone } = usage()
    expect(parseUsage(noTimezone)).toBeNull()
    expect(parseUsage({ ...usage(), devices: null })).toBeNull()
    expect(parseUsage({ ...usage(), from: 0 })).toBeNull()
  })

  it('rejects malformed nesting anywhere in a device entry', () => {
    expect(parseUsage(usage({ devices: [{ device_id: 'broken' } as never] }))).toBeNull()
    // totals is a Go value type: null is not a legal payload.
    expect(
      parseUsage(usage({ devices: [deviceUsage({ totals: null as never })] })),
    ).toBeNull()
    expect(
      parseUsage(
        usage({
          devices: [deviceUsage({ apps: [{ app: 'VS Code', seconds: 1 } as never] })],
        }),
      ),
    ).toBeNull()
    expect(
      parseUsage(
        usage({
          devices: [
            deviceUsage({
              apps: [
                {
                  app: 'VS Code',
                  seconds: 1,
                  activities: [{ description: '在写代码' } as never],
                },
              ],
            }),
          ],
        }),
      ),
    ).toBeNull()
    expect(
      parseUsage(usage({ devices: [deviceUsage({ hourly: [{ hour: 14 } as never] })] })),
    ).toBeNull()
    expect(
      parseUsage(
        usage({
          devices: [
            deviceUsage({ hourly: null, daily: [{ date: '2026-07-30' } as never] }),
          ],
        }),
      ),
    ).toBeNull()
  })

  it('rejects hourly/daily values that are neither null nor an array', () => {
    expect(
      parseUsage(usage({ devices: [deviceUsage({ hourly: undefined as never })] })),
    ).toBeNull()
    expect(
      parseUsage(usage({ devices: [deviceUsage({ daily: 0 as never })] })),
    ).toBeNull()
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
