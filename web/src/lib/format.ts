/**
 * Pure formatting helpers. No React, no fetch — see the frontend directory
 * spec. Every user-visible string in the UI that needs shaping is produced
 * here so wording stays consistent across components.
 */

import type { DeviceType, NetworkType } from '@/types/contract'

const MINUTE = 60
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

/**
 * Relative time in Chinese, e.g. "3 分钟前". `now` is injectable so this stays
 * testable without faking the clock.
 *
 * Unparsable input yields "未知". Timestamps slightly in the future (client
 * and server clocks never agree perfectly) are treated as "刚刚" rather than
 * rendered as negative.
 */
export function timeAgo(iso: string, now: Date = new Date()): string {
  const then = Date.parse(iso)
  if (Number.isNaN(then)) return '未知'

  const seconds = Math.floor((now.getTime() - then) / 1000)
  if (seconds < 10) return '刚刚'
  if (seconds < MINUTE) return `${seconds} 秒前`
  if (seconds < HOUR) return `${Math.floor(seconds / MINUTE)} 分钟前`
  if (seconds < DAY) return `${Math.floor(seconds / HOUR)} 小时前`
  return `${Math.floor(seconds / DAY)} 天前`
}

/** Idle duration in Chinese, e.g. "已挂机 12 分钟" reads as "12 分钟". */
export function formatIdle(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '未知'
  if (seconds < MINUTE) return `${Math.floor(seconds)} 秒`
  if (seconds < HOUR) return `${Math.floor(seconds / MINUTE)} 分钟`
  return `${Math.floor(seconds / HOUR)} 小时`
}

/**
 * Accumulated usage, e.g. "4 小时 32 分" / "12 分" / "38 秒".
 *
 * Seconds only show below one minute: a usage total is a sum of many buckets,
 * so "1 小时 3 分 7 秒" is noise. Hours are never capped — a 30-day total
 * legitimately reads "128 小时". Non-finite and negative input yields "未知",
 * same as formatIdle.
 */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '未知'
  const total = Math.floor(seconds)
  if (total < MINUTE) return `${total} 秒`
  if (total < HOUR) return `${Math.floor(total / MINUTE)} 分`
  const hours = Math.floor(total / HOUR)
  const minutes = Math.floor((total % HOUR) / MINUTE)
  return minutes === 0 ? `${hours} 小时` : `${hours} 小时 ${minutes} 分`
}

/** Hour-slot label for the "today" chart: 14 -> "14 时". */
export function formatHour(hour: number): string {
  if (!Number.isFinite(hour)) return '未知'
  return `${Math.trunc(hour)} 时`
}

/**
 * Day-slot label for the 7d / 30d chart: "2026-07-30" -> "7/30". Anything that
 * is not the server's `YYYY-MM-DD` is shown as-is rather than turned into
 * "NaN/NaN".
 */
export function formatDay(date: string): string {
  const match = /^\d{4}-(\d{2})-(\d{2})$/.exec(date)
  if (match === null) return date
  return `${Number(match[1])}/${Number(match[2])}`
}

const NETWORK_LABELS: Record<string, string> = {
  wifi: 'Wi-Fi',
  cellular: '移动网络',
  ethernet: '有线',
  offline: '无网络',
}

/**
 * Network label. Unknown values fall back to the raw string: the backend does
 * not constrain this field, and showing what a device reported beats hiding it.
 */
export function networkLabel(network: NetworkType | null): string | null {
  if (network === null) return null
  return NETWORK_LABELS[network] ?? network
}

const DEVICE_TYPE_LABELS: Record<string, string> = {
  windows: '电脑',
  android: '手机',
}

/** Device kind label, with a generic fallback for kinds added later. */
export function deviceTypeLabel(deviceType: DeviceType): string {
  return DEVICE_TYPE_LABELS[deviceType] ?? '设备'
}
