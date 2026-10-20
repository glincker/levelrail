import { afterEach, describe, expect, it } from 'vitest'
import { formatAbsolute, setPreferredTimeZone } from './dateFormat'

describe('formatAbsolute', () => {
  afterEach(() => {
    setPreferredTimeZone(undefined)
  })

  it('returns empty for invalid input', () => {
    expect(formatAbsolute('nope')).toBe('')
  })

  it('honours the preferred timezone', () => {
    const iso = '2026-01-01T00:30:00Z'
    setPreferredTimeZone('Pacific/Auckland')
    const nz = formatAbsolute(iso)
    setPreferredTimeZone('America/Los_Angeles')
    expect(formatAbsolute(iso)).not.toBe(nz)
  })

  it('falls back on an invalid zone', () => {
    setPreferredTimeZone('Not/AZone')
    expect(formatAbsolute('2026-01-01T00:30:00Z')).not.toBe('')
  })
})
