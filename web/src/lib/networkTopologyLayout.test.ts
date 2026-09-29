import { describe, expect, it } from 'vitest'
import {
  groupIntoZones,
  hasMeshAddress,
  resolveZoneKey,
  zoneLabel,
} from './networkTopologyLayout'
import type { NetworkTopologyResponse } from '../types/networkTopology'

const nodeA = {
  id: 'node_a',
  name: 'alpha',
  region: 'hetzner-fsn1',
  status: 'online',
  schedulable: true,
  mesh_address: '100.64.0.2',
  is_local: false,
}
const nodeB = {
  id: 'node_b',
  name: 'beta',
  status: 'online',
  schedulable: true,
  mesh_address: '100.64.0.3',
  is_local: true,
}

describe('zoneLabel', () => {
  it('prefers the region label', () => {
    expect(zoneLabel(nodeA)).toBe('hetzner-fsn1')
  })
  it('falls back to the node name when no region is set', () => {
    expect(zoneLabel(nodeB)).toBe('beta')
  })
})

describe('resolveZoneKey', () => {
  it('returns the placement node id directly when it is a known node', () => {
    expect(resolveZoneKey('node_a', [nodeA, nodeB])).toBe('node_a')
  })
  it('resolves an empty node id to the local node', () => {
    expect(resolveZoneKey('', [nodeA, nodeB])).toBe('node_b')
  })
  it('falls back to the unplaced sentinel for an unknown node id', () => {
    expect(resolveZoneKey('node_missing', [nodeA, nodeB])).toBe('__unplaced__')
  })
  it('falls back to the unplaced sentinel when nothing is local either', () => {
    expect(resolveZoneKey('', [nodeA])).toBe('__unplaced__')
  })
})

describe('hasMeshAddress', () => {
  it.each([
    [{ mesh_address: '100.64.0.2' }, true],
    [{ mesh_address: '' }, false],
    [{}, false],
  ])('%j -> %s', (entry, want) => {
    expect(hasMeshAddress(entry)).toBe(want)
  })
})

describe('groupIntoZones', () => {
  const topology: NetworkTopologyResponse = {
    zone: 'acme.internal',
    mesh_enabled: true,
    nodes: [nodeA, nodeB],
    apps: [
      { name: 'web', node_id: 'node_a', mesh_address: '100.64.0.2' },
      { name: 'worker', node_id: '', mesh_address: '100.64.0.3' },
      { name: 'orphan', node_id: 'node_missing' },
    ],
    databases: [{ name: 'main', engine: 'postgres', node_id: 'node_a' }],
    load_balancers: [{ service: 'web', algorithm: 'round_robin' }],
    connections: [{ app: 'web', database: 'main', env_var: 'DATABASE_URL' }],
  }

  it('creates one zone per node plus an unplaced zone for the rest', () => {
    const zones = groupIntoZones(topology)
    expect(zones.map((z) => z.key)).toEqual([
      'node_a',
      'node_b',
      '__unplaced__',
    ])
  })

  it('places each app, database, and load balancer in its resolved zone', () => {
    const zones = groupIntoZones(topology)
    const byKey = new Map(zones.map((z) => [z.key, z]))

    expect(byKey.get('node_a')?.apps.map((a) => a.name)).toEqual(['web'])
    expect(byKey.get('node_a')?.databases.map((d) => d.name)).toEqual(['main'])
    expect(byKey.get('node_a')?.loadBalancerByApp.get('web')?.algorithm).toBe(
      'round_robin',
    )
    expect(byKey.get('node_b')?.apps.map((a) => a.name)).toEqual(['worker'])
    expect(byKey.get('__unplaced__')?.apps.map((a) => a.name)).toEqual([
      'orphan',
    ])
  })

  it('omits the unplaced zone when nothing needs it', () => {
    const clean: NetworkTopologyResponse = {
      ...topology,
      apps: topology.apps.filter((a) => a.name !== 'orphan'),
    }
    const zones = groupIntoZones(clean)
    expect(zones.some((z) => z.key === '__unplaced__')).toBe(false)
  })
})
