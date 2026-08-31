import { describe, expect, it } from 'vitest'
import { computeInheritedEnv, inheritedOnlyEnv } from './envProvenance'

describe('computeInheritedEnv', () => {
  it('returns nothing when no tier defines any keys', () => {
    expect(computeInheritedEnv({})).toEqual({})
  })

  it('tags a key with the only tier that defines it', () => {
    const got = computeInheritedEnv({ organization: { LOG_LEVEL: 'info' } })
    expect(got).toEqual({ LOG_LEVEL: { tier: 'organization', value: 'info' } })
  })

  it('resolves a key defined at multiple tiers to the highest-priority one below the app, mirroring resolveEnv order', () => {
    const got = computeInheritedEnv({
      organization: { LOG_LEVEL: 'info' },
      project: { LOG_LEVEL: 'debug' },
      environment: { LOG_LEVEL: 'trace' },
    })
    expect(got.LOG_LEVEL).toEqual({ tier: 'environment', value: 'trace' })
  })

  it('prefers project over organization when environment does not define the key', () => {
    const got = computeInheritedEnv({
      organization: { LOG_LEVEL: 'info' },
      project: { LOG_LEVEL: 'debug' },
    })
    expect(got.LOG_LEVEL).toEqual({ tier: 'project', value: 'debug' })
  })
})

describe('inheritedOnlyEnv', () => {
  it('excludes keys the app already sets itself', () => {
    const got = inheritedOnlyEnv(
      { organization: { LOG_LEVEL: 'info', SHARED: 'x' } },
      { LOG_LEVEL: 'own-value' },
    )
    expect(got).toEqual([{ key: 'SHARED', value: 'x', tier: 'organization' }])
  })

  it('sorts by key', () => {
    const got = inheritedOnlyEnv(
      { organization: { ZETA: '1', ALPHA: '2' } },
      undefined,
    )
    expect(got.map((r) => r.key)).toEqual(['ALPHA', 'ZETA'])
  })

  it('returns an empty list when the app already defines every shared key', () => {
    const got = inheritedOnlyEnv(
      { organization: { LOG_LEVEL: 'info' } },
      { LOG_LEVEL: 'own-value' },
    )
    expect(got).toEqual([])
  })
})
