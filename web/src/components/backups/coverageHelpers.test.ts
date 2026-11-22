import { describe, expect, it } from 'vitest'
import {
  coverageBadgeVariant,
  coverageHeadline,
  restorableApps,
} from './coverageHelpers'

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

describe('restorableApps', () => {
  const row = (
    kind: 'volume' | 'database',
    app: string | undefined,
    count: number,
  ) => ({
    kind,
    app_name: app,
    name: 'x',
    state: 'verified' as const,
    primary_action: 'restore' as const,
    total_bytes: 0,
    backup_count: count,
    encrypted: false,
  })

  it('keeps only apps with a backed up volume, deduplicated and sorted', () => {
    const rows = [
      row('volume', 'web', 2),
      row('volume', 'api', 1),
      row('volume', 'web', 1),
      row('volume', 'empty', 0),
      row('database', 'db-app', 3),
    ]
    expect(restorableApps(rows)).toEqual(['api', 'web'])
  })
})
