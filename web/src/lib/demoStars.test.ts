import { describe, expect, it } from 'vitest'
import { formatStars } from './demoStars'

describe('formatStars', () => {
  it('formats thousands and keeps small counts plain', () => {
    expect(formatStars(0)).toBe('0')
    expect(formatStars(987)).toBe('987')
    expect(formatStars(1000)).toBe('1k')
    expect(formatStars(1234)).toBe('1.2k')
    expect(formatStars(1999)).toBe('1.9k')
    expect(formatStars(12500)).toBe('12.5k')
    expect(formatStars(2_300_000)).toBe('2.3M')
  })

  it('returns null when the count is absent or invalid', () => {
    expect(formatStars(undefined)).toBeNull()
    expect(formatStars(-1)).toBeNull()
    expect(formatStars(Number.NaN)).toBeNull()
  })
})
