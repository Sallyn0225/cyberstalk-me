import { describe, expect, it } from 'vitest'

import {
  deviceTypeLabel,
  formatIdle,
  networkLabel,
  timeAgo,
} from '@/lib/format'

const NOW = new Date('2026-07-29T12:00:00Z')

/** Builds an ISO timestamp `seconds` before NOW. */
function ago(seconds: number): string {
  return new Date(NOW.getTime() - seconds * 1000).toISOString()
}

describe('timeAgo', () => {
  it('says 刚刚 within the first 10 seconds', () => {
    expect(timeAgo(ago(0), NOW)).toBe('刚刚')
    expect(timeAgo(ago(9), NOW)).toBe('刚刚')
  })

  it('counts seconds up to a minute', () => {
    expect(timeAgo(ago(10), NOW)).toBe('10 秒前')
    expect(timeAgo(ago(59), NOW)).toBe('59 秒前')
  })

  it('counts minutes up to an hour', () => {
    expect(timeAgo(ago(60), NOW)).toBe('1 分钟前')
    expect(timeAgo(ago(3599), NOW)).toBe('59 分钟前')
  })

  it('counts hours up to a day', () => {
    expect(timeAgo(ago(3600), NOW)).toBe('1 小时前')
    expect(timeAgo(ago(86399), NOW)).toBe('23 小时前')
  })

  it('counts days beyond that', () => {
    expect(timeAgo(ago(86400), NOW)).toBe('1 天前')
    expect(timeAgo(ago(86400 * 9), NOW)).toBe('9 天前')
  })

  it('treats future timestamps as 刚刚 instead of negative time', () => {
    expect(timeAgo(new Date(NOW.getTime() + 30_000).toISOString(), NOW)).toBe(
      '刚刚',
    )
  })

  it('returns 未知 for unparsable input', () => {
    expect(timeAgo('not a date', NOW)).toBe('未知')
    expect(timeAgo('', NOW)).toBe('未知')
  })
})

describe('formatIdle', () => {
  it('formats seconds, minutes and hours', () => {
    expect(formatIdle(0)).toBe('0 秒')
    expect(formatIdle(59)).toBe('59 秒')
    expect(formatIdle(60)).toBe('1 分钟')
    expect(formatIdle(3599)).toBe('59 分钟')
    expect(formatIdle(3600)).toBe('1 小时')
    expect(formatIdle(86400)).toBe('24 小时')
  })

  it('returns 未知 for negative or non-finite input', () => {
    expect(formatIdle(-1)).toBe('未知')
    expect(formatIdle(Number.NaN)).toBe('未知')
    expect(formatIdle(Number.POSITIVE_INFINITY)).toBe('未知')
  })
})

describe('networkLabel', () => {
  it('maps known kinds and passes unknown ones through', () => {
    expect(networkLabel('wifi')).toBe('Wi-Fi')
    expect(networkLabel('cellular')).toBe('移动网络')
    expect(networkLabel('ethernet')).toBe('有线')
    expect(networkLabel('offline')).toBe('无网络')
    // The backend does not constrain this field, so unknown values must not
    // disappear silently.
    expect(networkLabel('bluetooth' as never)).toBe('bluetooth')
  })

  it('returns null when the device reported no network', () => {
    expect(networkLabel(null)).toBeNull()
  })
})

describe('deviceTypeLabel', () => {
  it('maps known kinds and falls back for future ones', () => {
    expect(deviceTypeLabel('windows')).toBe('电脑')
    expect(deviceTypeLabel('android')).toBe('手机')
    expect(deviceTypeLabel('linux' as never)).toBe('设备')
  })
})
