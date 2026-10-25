import { describe, expect, it } from 'vitest'
import { defaultScheduleWhen, validateScheduleAt } from './scheduleAt'

const NOW = new Date('2026-12-01T10:00:00').getTime()

describe('validateScheduleAt', () => {
  it.each([
    ['', 'required'],
    ['   ', 'required'],
    ['not a date', 'invalid'],
    ['2026-12-01T09:59', 'past'],
    ['2026-12-01T10:00', 'past'],
    ['2026-12-01T10:01', null],
    ['2027-01-01T03:00', null],
  ])('%j gives %s', (value, error) => {
    expect(validateScheduleAt(value, NOW).error).toBe(error)
  })

  it('returns a UTC ISO string for a valid pick', () => {
    const { iso } = validateScheduleAt('2026-12-01T12:30', NOW)
    expect(iso).toBe(new Date('2026-12-01T12:30').toISOString())
  })
})

describe('defaultScheduleWhen', () => {
  it('suggests the first whole hour at least an hour away', () => {
    const now = new Date('2026-12-01T10:20:00').getTime()
    expect(defaultScheduleWhen(now)).toBe('2026-12-01T12:00')
  })
})
