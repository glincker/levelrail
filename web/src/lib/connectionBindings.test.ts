import { describe, expect, it } from 'vitest'
import type { Connection } from '../types/connections'
import {
  canFillRole,
  connectionsForRole,
  isBindingRole,
  rolesForConnection,
} from './connectionBindings'

function conn(ref: string, capabilities: string[] | null): Connection {
  return {
    ref,
    kind: 'git',
    provider: 'github',
    label: ref,
    auth_method: 'token',
    capabilities,
    bound_count: 0,
    scope_type: 'instance',
    health: 'ok',
    expiring_soon: false,
  }
}

describe('connectionBindings', () => {
  it('limits roles to what the capabilities allow', () => {
    expect(rolesForConnection(conn('a', ['source', 'status']))).toEqual([
      'source',
      'deploy_report',
    ])
    expect(rolesForConnection(conn('b', null))).toEqual([])
  })

  it('filters connections for a role and knows role names', () => {
    const registry = conn('a', ['registry_pull'])
    const dns = conn('b', ['dns'])
    const list = [registry, dns]
    expect(connectionsForRole(list, 'dns').map((c) => c.ref)).toEqual(['b'])
    expect(canFillRole(registry, 'dns')).toBe(false)
    expect(isBindingRole('notify')).toBe(true)
    expect(isBindingRole('bogus')).toBe(false)
  })
})
