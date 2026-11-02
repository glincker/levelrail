import { describe, expect, it } from 'vitest'
import { clampRetentionDays, estimateRetention } from './parkedRetention'

const now = new Date('2026-11-02T00:00:00Z')
const day = (n: number) =>
  new Date(now.getTime() - n * 86_400_000).toISOString()

const parked = [
  { domain: 'new.com', kind: 'parked' as const, parked_at: day(3) },
  { domain: 'mid.com', kind: 'parked' as const, parked_at: day(40) },
  { domain: 'old.com', kind: 'parked' as const, parked_at: day(100) },
  { domain: 'none.com', kind: 'parked' as const },
]

describe('estimateRetention', () => {
  it('counts domains at or past the cutoff, oldest first', () => {
    const e = estimateRetention(parked, 30, 10, now)
    expect(e.candidates.map((c) => c.domain)).toEqual(['old.com', 'mid.com'])
    expect(e.total).toBe(2)
  })
  it('caps the first run at the batch limit', () => {
    const e = estimateRetention(parked, 30, 1, now)
    expect(e.total).toBe(2)
    expect(e.firstRun.map((c) => c.domain)).toEqual(['old.com'])
  })
  it('counts a domain exactly at the cutoff', () => {
    expect(estimateRetention(parked, 40, 10, now).total).toBe(2)
  })
  it('returns nothing for zero days or unknown ages', () => {
    expect(estimateRetention(parked, 0, 10, now).total).toBe(0)
    expect(estimateRetention(parked, 90, 10, now).total).toBe(1)
  })
})

describe('clampRetentionDays', () => {
  it('keeps values in range', () => {
    expect(clampRetentionDays(-4)).toBe(0)
    expect(clampRetentionDays(9999)).toBe(3650)
    expect(clampRetentionDays(12.7)).toBe(12)
    expect(clampRetentionDays(Number.NaN)).toBe(0)
  })
})
