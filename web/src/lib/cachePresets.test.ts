import { describe, expect, it } from 'vitest'
import {
  NEXTJS_VARY,
  canVaryOn,
  nextjsPresetRules,
  withNextjsPreset,
  withVary,
} from './cachePresets'

describe('cache presets', () => {
  it('adds the Next.js rules once and turns caching on', () => {
    const once = withNextjsPreset({ enabled: false, rules: [] })
    expect(once.enabled).toBe(true)
    expect(once.rules).toHaveLength(2)
    expect(once.rules[1]?.vary).toEqual(NEXTJS_VARY)
    expect(withNextjsPreset(once).rules).toHaveLength(2)
  })

  it('keeps the preset valid for the server limits', () => {
    for (const r of nextjsPresetRules()) {
      expect((r.vary ?? []).length).toBeLessThanOrEqual(8)
    }
  })

  it('extends existing rules and leaves never rules alone', () => {
    const spec = {
      enabled: true,
      rules: [
        {
          match: { kind: 'prefix' as const, path: '/a' },
          ttl_seconds: 5,
          vary: ['Rsc'],
        },
        {
          match: { kind: 'prefix' as const, path: '/b' },
          ttl_seconds: 0,
          never: true,
        },
      ],
    }
    const out = withVary(spec, ['rsc', 'next-router-state-tree'], [])
    expect(out.rules[0]?.vary).toEqual(['Rsc', 'next-router-state-tree'])
    expect(out.rules[1]?.vary).toBeUndefined()
  })

  it('falls back to the given rules when nothing can be extended', () => {
    const out = withVary(
      { enabled: false, rules: [] },
      ['rsc'],
      nextjsPresetRules(),
    )
    expect(out.enabled).toBe(true)
    expect(out.rules).toHaveLength(2)
  })

  it('refuses headers the cache cannot vary on', () => {
    expect(canVaryOn('Cookie')).toBe(false)
    expect(canVaryOn('*')).toBe(false)
    expect(canVaryOn('Rsc')).toBe(true)
  })
})
