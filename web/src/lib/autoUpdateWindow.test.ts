import { describe, expect, it } from 'vitest'
import {
  buildWindowCron,
  isValidTimezone,
  parseClock,
  parseWindowCron,
  windowEnd,
} from './autoUpdateWindow'

describe('autoUpdateWindow', () => {
  it('round-trips the friendly presets', () => {
    for (const cron of ['0 2 * * *', '30 3 * * 1-5', '0 4 * * 0,6']) {
      expect(buildWindowCron(parseWindowCron(cron), cron)).toBe(cron)
    }
  })

  it('keeps an unrecognised cron untouched', () => {
    const p = parseWindowCron('0 3 1 * *')
    expect(p.days).toBe('custom')
    expect(buildWindowCron(p, '0 3 1 * *')).toBe('0 3 1 * *')
  })

  it('wraps the end time past midnight', () => {
    expect(windowEnd(23, 30, 120)).toBe('01:30')
    expect(windowEnd(2, 0, 180)).toBe('05:00')
  })

  it('validates clock and time zone input', () => {
    expect(parseClock('25:00')).toBeNull()
    expect(parseClock('02:05')).toEqual({ hour: 2, minute: 5 })
    expect(isValidTimezone('Europe/Berlin')).toBe(true)
    expect(isValidTimezone('Mars/Base')).toBe(false)
  })
})
