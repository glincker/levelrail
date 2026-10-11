import { describe, expect, it } from 'vitest'
import { coverageBadgeVariant, coverageHeadline } from './coverageHelpers'

describe('coverageHeadline', () => {
  it.each([
    [{ total: 0, verified: 0, failing: 0, unprotected: 0 }, 'empty'],
    [{ total: 3, verified: 1, failing: 1, unprotected: 1 }, 'failing'],
    [{ total: 3, verified: 1, failing: 0, unprotected: 2 }, 'someUnprotected'],
    [{ total: 3, verified: 3, failing: 0, unprotected: 0 }, 'allVerified'],
    [{ total: 3, verified: 1, failing: 0, unprotected: 0 }, 'unverified'],
  ] as const)('%j is %s', (summary, want) => {
    expect(coverageHeadline(summary)).toBe(want)
  })
})

describe('coverageBadgeVariant', () => {
  it('never shows an unprotected resource as healthy', () => {
    expect(coverageBadgeVariant('unprotected')).toBe('destructive')
    expect(coverageBadgeVariant('failing')).toBe('destructive')
    expect(coverageBadgeVariant('never_ran')).toBe('warning')
    expect(coverageBadgeVariant('verified')).toBe('success')
  })
})
