import { describe, expect, it } from 'vitest'
import { canParkOrMove, isPrimaryDomain, moveTargets } from './domainLifecycle'

describe('domainLifecycle', () => {
  it('allows park and move for app domains only', () => {
    expect(canParkOrMove({ kind: 'app' })).toBe(true)
    expect(canParkOrMove({})).toBe(true)
    expect(canParkOrMove({ kind: 'standalone' })).toBe(false)
    expect(canParkOrMove({ kind: 'app', automatic: true })).toBe(false)
  })
  it('lists move targets without the owner', () => {
    expect(moveTargets([{ name: 'a' }, { name: 'b' }], 'a')).toEqual(['b'])
  })
  it('matches the primary domain case-insensitively', () => {
    expect(isPrimaryDomain('Glinr.com', 'glinr.com')).toBe(true)
    expect(isPrimaryDomain('a.com', undefined)).toBe(false)
  })
})
