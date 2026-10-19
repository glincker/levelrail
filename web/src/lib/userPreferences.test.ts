import { afterEach, describe, expect, it } from 'vitest'
import {
  formatUserDateTime,
  getUserTimezone,
  setUserTimezone,
} from './userPreferences'

afterEach(() => setUserTimezone(''))

describe('user timezone', () => {
  it('formats in the chosen zone', () => {
    const iso = '2026-01-01T00:30:00Z'
    expect(formatUserDateTime(iso, '', 'Asia/Tokyo')).not.toBe(
      formatUserDateTime(iso, '', 'America/Los_Angeles'),
    )
    setUserTimezone('Asia/Tokyo')
    expect(formatUserDateTime(iso)).toBe(
      formatUserDateTime(iso, '', 'Asia/Tokyo'),
    )
  })

  it('ignores an invalid zone and falls back to the browser', () => {
    setUserTimezone('Mars/Base')
    expect(getUserTimezone()).toBe('')
  })

  it('returns the fallback for a missing or broken timestamp', () => {
    expect(formatUserDateTime(undefined, 'never')).toBe('never')
    expect(formatUserDateTime('not a date', 'bad')).toBe('bad')
  })
})
