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

/**
 * Aggregation window asked of `GET /api/v1/usage`. Go: `UsageWindow = string`
 * with these known values, same as DeviceType — the guard below does not check
 * the value, so a window added later still parses.
 */
export type UsageWindow = 'today' | '7d' | '30d'

/** Per-state second counts for one device over the window. Go: value type. */
export interface UsageTotals {
  active_seconds: number
  idle_seconds: number
  locked_seconds: number
}

/** One mapped description's active time within an app. */
export interface ActivityUsage {
  description: string
  seconds: number
}

/** One app's active time. Idle and locked time never appear here. */
export interface AppUsage {
  app: string
  /** Active seconds only. Equals the sum of `activities`. */
  seconds: number
  activities: ActivityUsage[]
}

/** One hour of the local day. Hours with no usage are present with `seconds` 0. */
export interface HourUsage {
  hour: number
  seconds: number
  /** "" when `seconds` is 0. */
  top_app: string
}

/** One local day. Days with no usage are present with `seconds` 0. */
export interface DayUsage {
  /** `YYYY-MM-DD` in the response's `timezone`. */
  date: string
  seconds: number
  /** "" when `seconds` is 0. */
  top_app: string
}

/** One device's usage over the requested window. */
export interface DeviceUsage {
  device_id: string
  device_name: string
  device_type: DeviceType
  totals: UsageTotals
  /** Active-time ranking, descending. Empty when nothing was active. */
  apps: AppUsage[]
  /**
   * `hourly` is set for window "today" and null otherwise; `daily` is set for
   * "7d"/"30d" and null otherwise. Exactly one of them is non-null — Go ships
   * both as slices, so a nil one serializes to `null`.
   */
  hourly: HourUsage[] | null
  daily: DayUsage[] | null
}

/** Body of `GET /api/v1/usage`. Go: `shared.UsageResponse`. */
export interface UsageResponse {
  window: UsageWindow
  /** IANA name the window was computed in. */
  timezone: string
  /** RFC 3339, inclusive, UTC. */
  from: string
  /** RFC 3339, exclusive, UTC. */
  to: string
  devices: DeviceUsage[]
}

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

function isUsageTotals(value: unknown): value is UsageTotals {
  return (
    isRecord(value) &&
    typeof value.active_seconds === 'number' &&
    typeof value.idle_seconds === 'number' &&
    typeof value.locked_seconds === 'number'
  )
}

function isActivityUsage(value: unknown): value is ActivityUsage {
  return (
    isRecord(value) &&
    typeof value.description === 'string' &&
    typeof value.seconds === 'number'
  )
}

function isAppUsage(value: unknown): value is AppUsage {
  return (
    isRecord(value) &&
    typeof value.app === 'string' &&
    typeof value.seconds === 'number' &&
    Array.isArray(value.activities) &&
    value.activities.every(isActivityUsage)
  )
}

function isHourUsage(value: unknown): value is HourUsage {
  return (
    isRecord(value) &&
    typeof value.hour === 'number' &&
    typeof value.seconds === 'number' &&
    typeof value.top_app === 'string'
  )
}

function isDayUsage(value: unknown): value is DayUsage {
  return (
    isRecord(value) &&
    typeof value.date === 'string' &&
    typeof value.seconds === 'number' &&
    typeof value.top_app === 'string'
  )
}

function isDeviceUsage(value: unknown): value is DeviceUsage {
  return (
    isRecord(value) &&
    typeof value.device_id === 'string' &&
    typeof value.device_name === 'string' &&
    typeof value.device_type === 'string' &&
    isUsageTotals(value.totals) &&
    Array.isArray(value.apps) &&
    value.apps.every(isAppUsage) &&
    // Exactly one of the two is non-null in practice, but the guard only
    // checks "null or array": which one the server fills is its call, and the
    // UI picks whichever it got rather than insisting on a pairing.
    (value.hourly === null ||
      (Array.isArray(value.hourly) && value.hourly.every(isHourUsage))) &&
    (value.daily === null ||
      (Array.isArray(value.daily) && value.daily.every(isDayUsage)))
  )
}

export function isUsageResponse(value: unknown): value is UsageResponse {
  return (
    isRecord(value) &&
    // Structure only: `window` is not checked against the known values, so a
    // window the backend adds later still renders instead of blanking the tab.
    typeof value.window === 'string' &&
    typeof value.timezone === 'string' &&
    typeof value.from === 'string' &&
    typeof value.to === 'string' &&
    Array.isArray(value.devices) &&
    value.devices.every(isDeviceUsage)
  )
}

/**
 * Parses the `GET /api/v1/usage` body. Returns null when the payload is not the
 * expected envelope or any device entry is malformed; the caller shows the same
 * error state it uses for a failed request rather than rendering half a chart.
 */
export function parseUsage(value: unknown): UsageResponse | null {
  return isUsageResponse(value) ? value : null
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
