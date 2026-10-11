import { describe, expect, it } from 'vitest'
import { ageOf, daysUntil } from './trafficTime'

const NOW = Date.parse('2026-10-10T12:00:00Z')

describe('ageOf', () => {
  it.each([
    ['2026-10-10T11:59:46Z', { unit: 'seconds', count: 14 }],
    ['2026-10-10T11:57:00Z', { unit: 'minutes', count: 3 }],
    ['2026-10-10T09:00:00Z', { unit: 'hours', count: 3 }],
    ['2026-10-07T12:00:00Z', { unit: 'days', count: 3 }],
  ])('%s', (from, want) => {
    expect(ageOf(from, NOW)).toEqual(want)
  })

  it('clamps a future timestamp to zero seconds', () => {
    expect(ageOf('2026-10-10T12:00:30Z', NOW)).toEqual({
      unit: 'seconds',
      count: 0,
    })
  })

  it('returns null for garbage', () => {
    expect(ageOf('nope', NOW)).toBeNull()
  })
})

describe('daysUntil', () => {
  it('counts whole days', () => {
    expect(daysUntil('2026-12-31T12:00:00Z', NOW)).toBe(82)
  })
  it('is negative once expired', () => {
    expect(daysUntil('2026-10-08T12:00:00Z', NOW)).toBe(-2)
  })
  it('is null for garbage', () => {
    expect(daysUntil('x', NOW)).toBeNull()
  })
})
