import { describe, expect, it } from 'vitest'
import { hoursUntil, soonDaysFromSetting, tokenExpiry } from './tokenExpiry'

const NOW = Date.parse('2026-11-15T12:00:00Z')
const at = (ms: number) => new Date(NOW + ms).toISOString()
const H = 3_600_000
const D = 24 * H

describe('tokenExpiry', () => {
  it('has no state without an expiry', () => {
    expect(tokenExpiry(undefined, NOW, 14).state).toBe('none')
  })
  it('is ok beyond the soon window and counts days', () => {
    expect(tokenExpiry(at(30 * D), NOW, 14)).toEqual({
      state: 'ok',
      unit: 'days',
      count: 30,
    })
  })
  it('is soon inside the window', () => {
    expect(tokenExpiry(at(5 * D), NOW, 14).state).toBe('soon')
  })
  it('switches to hours under 48 hours', () => {
    expect(tokenExpiry(at(47 * H), NOW, 14)).toEqual({
      state: 'soon',
      unit: 'hours',
      count: 47,
    })
    expect(tokenExpiry(at(48 * H), NOW, 14).unit).toBe('days')
  })
  it('never shows zero hours while valid', () => {
    expect(tokenExpiry(at(60_000), NOW, 14).count).toBe(1)
  })
  it('is expired at or past the expiry', () => {
    expect(tokenExpiry(at(0), NOW, 14).state).toBe('expired')
    expect(tokenExpiry(at(-D), NOW, 14).state).toBe('expired')
  })
})

describe('soonDaysFromSetting', () => {
  it('takes the largest window from a list or a string', () => {
    expect(soonDaysFromSetting(['7', '14', '1'])).toBe(14)
    expect(soonDaysFromSetting('3:30:1')).toBe(30)
  })
  it('falls back to the default on junk', () => {
    expect(soonDaysFromSetting(undefined)).toBe(14)
  })
})

describe('hoursUntil', () => {
  it('rounds up and is zero once past', () => {
    expect(hoursUntil(at(90 * 60_000), NOW)).toBe(2)
    expect(hoursUntil(at(-1), NOW)).toBe(0)
    expect(hoursUntil(undefined, NOW)).toBe(0)
  })
})
