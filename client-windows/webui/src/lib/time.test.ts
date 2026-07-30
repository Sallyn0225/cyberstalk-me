import { describe, expect, it } from 'vitest'
import { isRecent, relativeTime } from './time'

const now = Date.parse('2026-07-30T12:00:00Z')
const ago = (seconds: number) => new Date(now - seconds * 1000).toISOString()

describe('relativeTime', () => {
  it('renders each band', () => {
    expect(relativeTime(ago(0), now)).toBe('刚刚')
    expect(relativeTime(ago(4), now)).toBe('刚刚')
    expect(relativeTime(ago(30), now)).toBe('30 秒前')
    expect(relativeTime(ago(59), now)).toBe('59 秒前')
    expect(relativeTime(ago(60), now)).toBe('1 分钟前')
    expect(relativeTime(ago(3599), now)).toBe('59 分钟前')
    expect(relativeTime(ago(3600), now)).toBe('1 小时前')
    expect(relativeTime(ago(86_400), now)).toBe('1 天前')
  })

  it('reads a slightly-future timestamp as "just now" rather than a negative', () => {
    // The agent and the browser share a clock, but not to the millisecond.
    expect(relativeTime(ago(-2), now)).toBe('刚刚')
  })

  it('renders nothing for a timestamp it cannot parse', () => {
    expect(relativeTime('', now)).toBe('')
    expect(relativeTime('not a date', now)).toBe('')
  })
})

describe('isRecent', () => {
  it('marks what just appeared', () => {
    expect(isRecent(ago(1), now)).toBe(true)
    expect(isRecent(ago(10), now)).toBe(false)
    expect(isRecent(ago(10), now, 20_000)).toBe(true)
  })

  it('is false for an unparseable timestamp', () => {
    expect(isRecent('nonsense', now)).toBe(false)
  })
})
