import { describe, expect, it } from 'vitest'

import { maxSeconds, sharePercent } from '@/lib/usage'

describe('sharePercent', () => {
  it('computes the share', () => {
    expect(sharePercent(50, 200)).toBe(25)
    expect(sharePercent(200, 200)).toBe(100)
  })

  it('returns 0 when the denominator is 0 — the empty-window case', () => {
    expect(sharePercent(0, 0)).toBe(0)
    expect(sharePercent(10, 0)).toBe(0)
    expect(sharePercent(10, -5)).toBe(0)
  })

  it('returns 0 for non-finite input instead of NaN or Infinity', () => {
    expect(sharePercent(Number.NaN, 100)).toBe(0)
    expect(sharePercent(10, Number.NaN)).toBe(0)
    expect(sharePercent(Number.POSITIVE_INFINITY, 100)).toBe(0)
    expect(sharePercent(10, Number.POSITIVE_INFINITY)).toBe(0)
  })

  it('clamps above 100 and never goes negative', () => {
    expect(sharePercent(300, 100)).toBe(100)
    expect(sharePercent(-10, 100)).toBe(0)
  })
})

describe('maxSeconds', () => {
  it('finds the largest value', () => {
    expect(maxSeconds([{ seconds: 3 }, { seconds: 90 }, { seconds: 12 }])).toBe(90)
  })

  it('returns 0 for an empty or all-zero list', () => {
    expect(maxSeconds([])).toBe(0)
    expect(maxSeconds([{ seconds: 0 }, { seconds: 0 }])).toBe(0)
  })

  it('ignores non-finite entries', () => {
    expect(maxSeconds([{ seconds: Number.NaN }, { seconds: 5 }])).toBe(5)
    expect(maxSeconds([{ seconds: Number.POSITIVE_INFINITY }, { seconds: 5 }])).toBe(5)
  })
})
