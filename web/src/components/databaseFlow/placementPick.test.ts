import { describe, expect, it } from 'vitest'
import {
  AUTO_NODE,
  ineligibleReason,
  pickAutoNode,
  placementBlocksCreate,
  resolvePlacementNodeId,
  type PlacementNode,
} from './placementPick'

function node(over: Partial<PlacementNode> & { id: string }): PlacementNode {
  return {
    name: over.id,
    isLocal: false,
    online: true,
    schedulable: true,
    outdated: false,
    failing: false,
    workloads: 0,
    ...over,
  }
}

describe('pickAutoNode', () => {
  it('prefers the idle node over a loaded one', () => {
    const picked = pickAutoNode([
      node({ id: 'busy', cpuPercent: 80, diskFreePercent: 90 }),
      node({ id: 'idle', cpuPercent: 5, diskFreePercent: 50 }),
    ])
    expect(picked?.id).toBe('idle')
  })

  it('skips offline, cordoned, no docker and low disk nodes', () => {
    const list = [
      node({ id: 'a', online: false }),
      node({ id: 'b', schedulable: false }),
      node({ id: 'c', docker: false }),
      node({ id: 'd', diskFreePercent: 2 }),
      node({ id: 'ok', cpuPercent: 60, diskFreePercent: 40 }),
    ]
    expect(pickAutoNode(list, 15)?.id).toBe('ok')
    expect(ineligibleReason(list[2] as PlacementNode, 15)).toBe('noDocker')
    expect(ineligibleReason(list[3] as PlacementNode, 15)).toBe('lowDisk')
  })

  it('returns null when nothing is eligible', () => {
    expect(pickAutoNode([node({ id: 'a', online: false })])).toBeNull()
  })

  it('ranks known readings ahead of unknown ones', () => {
    const picked = pickAutoNode([
      node({ id: 'unknown' }),
      node({ id: 'known', cpuPercent: 90 }),
    ])
    expect(picked?.id).toBe('known')
  })

  it('breaks ties the same way whatever the list order', () => {
    const a = node({ id: 'n1', name: 'alpha', workloads: 2 })
    const b = node({ id: 'n2', name: 'beta', workloads: 2 })
    const c = node({ id: 'n3', name: 'gamma', workloads: 1 })
    expect(pickAutoNode([a, b, c])?.id).toBe('n3')
    expect(pickAutoNode([b, a])?.id).toBe('n1')
    expect(pickAutoNode([a, b])?.id).toBe('n1')
  })
})

describe('resolvePlacementNodeId', () => {
  const list = [
    node({ id: 'local', isLocal: true, cpuPercent: 50 }),
    node({ id: 'edge', cpuPercent: 10 }),
  ]
  it('maps each choice to a node id', () => {
    expect(resolvePlacementNodeId('local', '', list)).toBe('local')
    expect(resolvePlacementNodeId('node', AUTO_NODE, list)).toBe('edge')
    expect(resolvePlacementNodeId('node', 'x', list)).toBe('x')
    expect(resolvePlacementNodeId('cloud', '', list)).toBeUndefined()
  })
  it('blocks create for choices without a node yet', () => {
    expect(placementBlocksCreate('cloud')).toBe(true)
    expect(placementBlocksCreate('host')).toBe(true)
    expect(placementBlocksCreate('node')).toBe(false)
  })
})
