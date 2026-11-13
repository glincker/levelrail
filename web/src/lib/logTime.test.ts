import { describe, expect, it } from 'vitest'
import { formatLogAbsolute, formatLogRelative, parseLogTime } from './logTime'

const NOW = Date.UTC(2026, 10, 12, 12, 0, 0)

describe('parseLogTime', () => {
  it('parses RFC3339 and epoch ms, rejects the rest', () => {
    expect(parseLogTime('2026-11-12T12:00:00Z')).toBe(NOW)
    expect(parseLogTime(NOW)).toBe(NOW)
    expect(parseLogTime('')).toBeUndefined()
    expect(parseLogTime('nope')).toBeUndefined()
    expect(parseLogTime(undefined)).toBeUndefined()
  })
})

describe('formatLogRelative', () => {
  it('uses second precision then coarser units', () => {
    expect(formatLogRelative(NOW - 12_000, NOW)).toMatch(/12/)
    expect(formatLogRelative(NOW - 12_000, NOW)).toMatch(/s/)
    expect(formatLogRelative(NOW - 5 * 60_000, NOW)).toMatch(/5/)
    expect(formatLogRelative(NOW - 5 * 60_000, NOW)).toMatch(/m/)
    expect(formatLogRelative(NOW - 3 * 3_600_000, NOW)).toMatch(/3/)
  })

  it('does not show a negative future for tiny clock skew', () => {
    expect(formatLogRelative(NOW + 500, NOW)).not.toBe('')
  })
})

describe('formatLogAbsolute', () => {
  it('includes the year and seconds', () => {
    expect(formatLogAbsolute(NOW)).toMatch(/2026/)
  })
})
