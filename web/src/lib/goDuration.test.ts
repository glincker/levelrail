import { describe, expect, it } from 'vitest'
import { formatGoDuration, parseGoDuration } from './goDuration'

describe('goDuration', () => {
  it('parses Go duration strings', () => {
    expect(parseGoDuration('6h0m0s')).toBe(6 * 3_600_000)
    expect(parseGoDuration('15m')).toBe(15 * 60_000)
    expect(parseGoDuration('1h30m')).toBe(90 * 60_000)
    expect(parseGoDuration('soon')).toBeNull()
  })

  it('formats for the locale', () => {
    expect(formatGoDuration('6h0m0s', 'en')).toMatch(/^6 hours?$/)
    expect(formatGoDuration('15m', 'en')).toMatch(/^15 minutes?$/)
    expect(formatGoDuration('nope', 'en')).toBe('nope')
  })
})
