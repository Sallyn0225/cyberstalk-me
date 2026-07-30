/**
 * TypeScript mirror of the Go wire contract in `shared/contract.go`.
 *
 * Field names match the Go JSON tags exactly (snake_case). This file is the
 * only place API payload shapes are declared — when `shared/contract.go`
 * changes, this file changes in the same task.
 *
 * Nullability follows the Go types, not convenience: a Go pointer field is
 * `| null` here, a Go value field is not nullable.
 */

/** Known device kinds. The backend rejects reports with any other value. */
export type DeviceType = 'windows' | 'android'

/** Known network kinds. Go: `*NetworkType`, so `null` means unreported. */
export type NetworkType = 'wifi' | 'cellular' | 'ethernet' | 'offline'

/** Sanitized description of what the user is doing. Go: value type. */
export interface Activity {
  app: string
  description: string
  idle: boolean
  idle_seconds: number
  /**
   * No foreground window at all (lock screen / session switch). Go: a plain
   * `bool`, but optional here because an agent older than the field omits it —
   * see the guard below. Read it as `activity.locked === true`.
   */
  locked?: boolean
}

/** Go: `*Battery`; `level` is `*int` and is null when the OS can't report it. */
export interface Battery {
  level: number | null
  charging: boolean
}

/** Server projection of a device, as delivered by snapshot and SSE events. */
export interface DeviceState {
  device_id: string
  device_name: string
  device_type: DeviceType
  /** Go value type — never null. */
  activity: Activity
  /** Null on machines without a battery (desktops). */
  battery: Battery | null
  network: NetworkType | null
  online: boolean
  /** RFC 3339, client clock — display/debug only. */
  reported_at: string
  /** RFC 3339, server clock — the basis of the online judgment. */
  last_seen_at: string
}

/** SSE payload. Go: `shared.Event`. */
export type StreamEvent =
  | { type: 'update'; device: DeviceState }
  | { type: 'offline'; device: DeviceState }

// --- Runtime guards -------------------------------------------------------
//
// The single trust boundary is "JSON off the wire". No schema library: the
// contract is narrow and ours. Guards check structure only, deliberately NOT
// the string-union values — if the backend later reports a new device or
// network kind, the UI should degrade to a generic label rather than drop the
// device entirely.

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isActivity(value: unknown): value is Activity {
  return (
    isRecord(value) &&
    typeof value.app === 'string' &&
    typeof value.description === 'string' &&
    typeof value.idle === 'boolean' &&
    typeof value.idle_seconds === 'number' &&
    // Optional on purpose. Requiring it would make every device behind an
    // older agent fail the guard and vanish from the page — the same reason
    // the string unions above are not checked.
    (value.locked === undefined || typeof value.locked === 'boolean')
  )
}

function isBattery(value: unknown): value is Battery {
  return (
    isRecord(value) &&
    (value.level === null || typeof value.level === 'number') &&
    typeof value.charging === 'boolean'
  )
}

export function isDeviceState(value: unknown): value is DeviceState {
  return (
    isRecord(value) &&
    typeof value.device_id === 'string' &&
    typeof value.device_name === 'string' &&
    typeof value.device_type === 'string' &&
    isActivity(value.activity) &&
    (value.battery === null || isBattery(value.battery)) &&
    (value.network === null || typeof value.network === 'string') &&
    typeof value.online === 'boolean' &&
    typeof value.reported_at === 'string' &&
    typeof value.last_seen_at === 'string'
  )
}

/**
 * Parses the `GET /api/v1/snapshot` body. The endpoint returns a bare array
 * of devices (not an envelope object). Returns null when the payload is not
 * an array or any entry is malformed; the caller warns and keeps the previous
 * state rather than blanking the page.
 */
export function parseSnapshot(value: unknown): DeviceState[] | null {
  if (!Array.isArray(value)) return null
  if (!value.every(isDeviceState)) return null
  return value as DeviceState[]
}

/**
 * Parses one SSE `data:` line. Returns null for anything malformed so a bad
 * event is dropped instead of crashing the page.
 */
export function parseStreamEvent(raw: string): StreamEvent | null {
  let data: unknown
  try {
    data = JSON.parse(raw)
  } catch {
    return null
  }
  if (!isRecord(data)) return null
  if (data.type !== 'update' && data.type !== 'offline') return null
  if (!isDeviceState(data.device)) return null
  return data as StreamEvent
}
