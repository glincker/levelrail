import { describe, expect, it } from 'vitest'
import {
  buildSeconds,
  deployStateKey,
  proxiedApps,
  repoLabel,
  splitDuration,
} from './overviewHelpers'

describe('overviewHelpers', () => {
  it('reduces a clone URL to owner/repo', () => {
    expect(repoLabel('https://github.com/acme/site.git')).toBe('acme/site')
    expect(repoLabel('git@github.com:acme/site.git')).toBe('acme/site')
    expect(repoLabel('not a url')).toBe('not a url')
  })

  it('measures build time only for finished deploys', () => {
    expect(buildSeconds('2026-01-01T00:00:00Z', '2026-01-01T00:03:03Z')).toBe(
      183,
    )
    expect(buildSeconds('2026-01-01T00:00:00Z', undefined)).toBeNull()
    expect(splitDuration(183)).toEqual({ minutes: 3, seconds: 3 })
  })

  it('falls back to unknown for statuses without a label', () => {
    expect(deployStateKey('succeeded')).toBe('succeeded')
    expect(deployStateKey('exotic')).toBe('unknown')
    expect(deployStateKey(undefined)).toBe('unknown')
  })

  it('lists each proxied app once', () => {
    const rule = (to: string) => ({
      source: 'ui',
      from: '/x',
      to,
      status: 200,
    })
    expect(
      proxiedApps([
        rule('app://api/v1'),
        rule('app://api/v2'),
        rule('/local'),
        rule('app://blog'),
      ]),
    ).toEqual(['api', 'blog'])
  })
})
