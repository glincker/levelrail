import { describe, expect, it } from 'vitest'
import { pollMs, resolveTierMs } from './pollTier'

describe('pollTier', () => {
  it('uses the defaults, ordered from fastest to slowest', () => {
    expect(pollMs('live')).toBeLessThan(pollMs('active'))
    expect(pollMs('active')).toBeLessThan(pollMs('steady'))
    expect(pollMs('steady')).toBeLessThan(pollMs('idle'))
  })

  it('takes a valid env override', () => {
    expect(resolveTierMs('steady', { VITE_POLL_STEADY_MS: '45000' })).toBe(
      45_000,
    )
  })

  it('ignores junk and values under one second', () => {
    expect(resolveTierMs('live', { VITE_POLL_LIVE_MS: 'x' })).toBe(5_000)
    expect(resolveTierMs('live', { VITE_POLL_LIVE_MS: '50' })).toBe(5_000)
  })
})
