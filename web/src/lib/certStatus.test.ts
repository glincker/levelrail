import { describe, expect, it } from 'vitest'
import { certExpiryLabel } from './certStatus'

const now = new Date('2026-09-23T12:00:00Z')

describe('certExpiryLabel', () => {
  it.each([
    ['2026-10-23T12:00:00Z', 'expires in 30 days'],
    ['2026-09-24T18:00:00Z', 'expires in 1 day'],
    ['2026-09-23T20:00:00Z', 'expires today'],
    ['2026-09-20T12:00:00Z', 'expired 3 days ago'],
    ['2026-09-22T06:00:00Z', 'expired 1 day ago'],
    ['2026-09-23T06:00:00Z', 'expired today'],
    ['garbage', ''],
  ])('%s -> %s', (notAfter, want) => {
    expect(certExpiryLabel(notAfter, now)).toBe(want)
  })
})
