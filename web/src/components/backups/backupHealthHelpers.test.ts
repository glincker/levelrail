import { describe, expect, it } from 'vitest'
import type { ResourceHealth } from '../../types/backupProtection'
import {
  DATABASES_GROUP,
  groupHealth,
  sortByState,
  stateBadgeVariant,
} from './backupHealthHelpers'

function resource(over: Partial<ResourceHealth>): ResourceHealth {
  return {
    kind: 'volume',
    app_name: 'web',
    resource_name: 'data',
    backup_count: 1,
    total_bytes: 1,
    encrypted: true,
    state: 'healthy',
    ...over,
  }
}

describe('groupHealth', () => {
  it('groups volumes under their app and databases last', () => {
    const groups = groupHealth([
      resource({
        kind: 'database',
        app_name: undefined,
        resource_name: 'main',
      }),
      resource({ app_name: 'zeta', resource_name: 'a' }),
      resource({ app_name: 'alpha', resource_name: 'a' }),
      resource({ app_name: 'alpha', resource_name: 'b' }),
    ])
    expect(groups.map((g) => g.key)).toEqual(['alpha', 'zeta', DATABASES_GROUP])
    expect(groups[0]?.resources).toHaveLength(2)
  })
})

describe('sortByState', () => {
  it('puts failing resources before healthy ones', () => {
    const sorted = sortByState([
      resource({ resource_name: 'a', state: 'healthy' }),
      resource({ resource_name: 'b', state: 'failing' }),
      resource({ resource_name: 'c', state: 'unverified' }),
    ])
    expect(sorted.map((r) => r.state)).toEqual([
      'failing',
      'unverified',
      'healthy',
    ])
  })
})

describe('stateBadgeVariant', () => {
  it('maps states to badge variants', () => {
    expect(stateBadgeVariant('healthy')).toBe('success')
    expect(stateBadgeVariant('failing')).toBe('destructive')
    expect(stateBadgeVariant('unverified')).toBe('warning')
    expect(stateBadgeVariant('unprotected')).toBe('muted')
  })
})
