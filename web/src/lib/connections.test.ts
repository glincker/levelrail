import { describe, expect, it } from 'vitest'
import {
  DEFAULT_FILTERS,
  attentionList,
  catalogDestination,
  effectiveHealth,
  expiryInfo,
  filterConnections,
  groupByKind,
  groupCatalog,
  needsAttention,
} from './connections'
import type { Connection } from '../types/connections'

const NOW = Date.parse('2026-11-15T12:00:00Z')
const day = (n: number) => new Date(NOW + n * 86_400_000).toISOString()

function conn(over: Partial<Connection> = {}): Connection {
  return {
    ref: 'git:1',
    kind: 'git',
    provider: 'github',
    label: 'acme',
    auth_method: 'token',
    capabilities: [],
    bound_count: 0,
    scope_type: 'instance',
    health: 'ok',
    expiring_soon: false,
    ...over,
  }
}

describe('expiryInfo', () => {
  it('counts whole days, rounding the last partial day up', () => {
    expect(expiryInfo(conn({ expires_at: day(3.2) }), NOW).days).toBe(4)
    expect(expiryInfo(conn({ expires_at: day(0.1) }), NOW).days).toBe(1)
  })
  it('goes negative once expired', () => {
    const info = expiryInfo(conn({ expires_at: day(-2.5) }), NOW)
    expect(info.expired).toBe(true)
    expect(info.days).toBe(-3)
  })
  it('has no days without an expiry and flags oauth auto renew', () => {
    expect(expiryInfo(conn(), NOW).days).toBeNull()
    expect(expiryInfo(conn({ auth_method: 'oauth' }), NOW).autoRenew).toBe(true)
    expect(expiryInfo(conn(), NOW).autoRenew).toBe(false)
  })
})

describe('health and attention', () => {
  it('turns a stale ok into expired when the date has passed', () => {
    expect(effectiveHealth(conn({ expires_at: day(-1) }), NOW)).toBe('expired')
    expect(effectiveHealth(conn({ expires_at: day(5) }), NOW)).toBe('ok')
  })
  it('flags broken and soon-to-expire token connections', () => {
    expect(needsAttention(conn({ health: 'needs_reconnect' }), NOW)).toBe(true)
    expect(needsAttention(conn({ expiring_soon: true }), NOW)).toBe(true)
    expect(needsAttention(conn(), NOW)).toBe(false)
  })
  it('ignores expiring oauth connections that renew themselves', () => {
    expect(
      needsAttention(conn({ auth_method: 'oauth', expiring_soon: true }), NOW),
    ).toBe(false)
  })
  it('sorts the strip worst first, then by days left', () => {
    const list = attentionList(
      [
        conn({ ref: 'a', expiring_soon: true, expires_at: day(2) }),
        conn({ ref: 'b', health: 'degraded' }),
        conn({ ref: 'c', health: 'expired' }),
      ],
      NOW,
    )
    expect(list.map((c) => c.ref)).toEqual(['c', 'b', 'a'])
  })
})

describe('filter and group', () => {
  const list = [
    conn({ ref: 'a', label: 'Alpha', kind: 'git' }),
    conn({ ref: 'b', label: 'Beta', kind: 'registry', scope_type: 'org' }),
    conn({ ref: 'c', label: 'Gamma', kind: 'git', health: 'degraded' }),
  ]
  it('filters by kind, scope, health and search', () => {
    expect(
      filterConnections(list, { ...DEFAULT_FILTERS, kind: 'git' }, NOW),
    ).toHaveLength(2)
    expect(
      filterConnections(list, { ...DEFAULT_FILTERS, scope: 'org' }, NOW),
    ).toHaveLength(1)
    expect(
      filterConnections(list, { ...DEFAULT_FILTERS, health: 'degraded' }, NOW),
    ).toHaveLength(1)
    expect(
      filterConnections(list, { ...DEFAULT_FILTERS, query: ' bet' }, NOW),
    ).toHaveLength(1)
  })
  it('groups by kind in a fixed order and skips empty kinds', () => {
    expect(groupByKind(list).map((g) => g.kind)).toEqual(['git', 'registry'])
  })
  it('groups the catalogue by category and routes unknown ones to settings', () => {
    const groups = groupCatalog([
      { id: 'a', category: 'git', name: 'A', connect_methods: null },
      { id: 'b', category: 'ci', name: 'B', connect_methods: ['token'] },
    ])
    expect(groups.map((g) => g.category)).toEqual(['git', 'ci'])
    expect(catalogDestination('git')).toBe('/settings/git-connections')
    expect(catalogDestination('ci')).toBe('/settings')
  })
})
