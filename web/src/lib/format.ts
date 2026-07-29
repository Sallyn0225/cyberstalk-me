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
