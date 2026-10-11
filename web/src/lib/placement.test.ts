import { describe, expect, it } from 'vitest'
import { resolvePlacement, type PlacementInput } from './placement'

const nodes = [
  { id: 'n_local', name: 'edge-1', is_local: true },
  { id: 'n_remote', name: 'worker-1', is_local: false },
]

const cases: [string, PlacementInput, string, string][] = [
  ['empty id is this server', {}, 'local', ''],
  [
    'server flag wins',
    { nodeId: 'x', nodeIsLocal: true, nodeName: 'h' },
    'local',
    'h',
  ],
  ['local node id from the list', { nodeId: 'n_local' }, 'local', 'edge-1'],
  [
    'remote by server name',
    { nodeId: 'n_remote', nodeName: 'w' },
    'remote',
    'w',
  ],
  ['remote by node list', { nodeId: 'n_remote' }, 'remote', 'worker-1'],
  ['unknown id', { nodeId: 'gone' }, 'unknown', 'gone'],
]

describe('resolvePlacement', () => {
  it.each(cases)('%s', (_label, input, kind, name) => {
    expect(resolvePlacement(input, nodes)).toEqual({ kind, name })
  })
})
