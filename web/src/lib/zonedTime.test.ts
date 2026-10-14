import { describe, expect, it } from 'vitest'
import {
  fromLocalInputValue,
  toLocalInputValue,
  wallTimeAt,
  wallTimeToInstant,
} from './zonedTime'

const NY = 'America/New_York'

describe('wallTimeToInstant', () => {
  const cases: Array<{
    name: string
    zone: string
    wall: [number, number, number, number, number]
    iso: string
  }> = [
    {
      name: 'UTC passthrough',
      zone: 'UTC',
      wall: [2026, 10, 14, 12, 0],
      iso: '2026-10-14T12:00:00.000Z',
    },
    {
      name: 'standard time offset',
      zone: NY,
      wall: [2026, 1, 15, 9, 0],
      iso: '2026-01-15T14:00:00.000Z',
    },
    {
      name: 'daylight time offset',
      zone: NY,
      wall: [2026, 7, 15, 9, 0],
      iso: '2026-07-15T13:00:00.000Z',
    },
    {
      name: 'spring forward gap resolves after the gap',
      zone: NY,
      wall: [2026, 3, 8, 2, 30],
      iso: '2026-03-08T07:30:00.000Z',
    },
    {
      name: 'fall back ambiguous hour takes the first occurrence',
      zone: NY,
      wall: [2026, 11, 1, 1, 30],
      iso: '2026-11-01T05:30:00.000Z',
    },
    {
      name: 'zone ahead of UTC across the date line',
      zone: 'Pacific/Auckland',
      wall: [2026, 10, 14, 0, 30],
      iso: '2026-10-13T11:30:00.000Z',
    },
  ]
  for (const c of cases) {
    it(c.name, () => {
      const [year, month, day, hour, minute] = c.wall
      const ms = wallTimeToInstant({ year, month, day, hour, minute }, c.zone)
      expect(new Date(ms).toISOString()).toBe(c.iso)
    })
  }
})

describe('local input values', () => {
  it('round trips a normal time', () => {
    const ms = fromLocalInputValue('2026-07-15T09:00', NY)
    expect(toLocalInputValue(ms, NY)).toBe('2026-07-15T09:00')
  })

  it('shows the post-gap wall time for a nonexistent time', () => {
    const ms = fromLocalInputValue('2026-03-08T02:30', NY)
    expect(toLocalInputValue(ms, NY)).toBe('2026-03-08T03:30')
  })

  it('rejects malformed values', () => {
    expect(fromLocalInputValue('', NY)).toBeNaN()
    expect(fromLocalInputValue('2026-07-15', NY)).toBeNaN()
  })

  it('reads midnight as hour 0, not 24', () => {
    const ms = Date.UTC(2026, 0, 1, 5, 0)
    expect(wallTimeAt(ms, NY).hour).toBe(0)
  })
})
