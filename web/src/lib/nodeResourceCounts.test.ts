import { describe, expect, it } from 'vitest'
import { countResourcesByNode } from './nodeResourceCounts'

describe('countResourcesByNode', () => {
  const nodes = [
    { id: 'l', is_local: true },
    { id: 'r', is_local: false },
  ]

  it('counts empty placement against the local node', () => {
    const got = countResourcesByNode(
      nodes,
      [{ node_id: '' }, { node_id: 'r' }, {}],
      [{ node_id: 'l' }, { node_id: 'gone' }],
    )
    expect(got.get('l')).toBe(3)
    expect(got.get('r')).toBe(1)
  })

  it('has no local bucket when no node is local', () => {
    const got = countResourcesByNode([{ id: 'r' }], [{}], [])
    expect(got.get('r')).toBe(0)
  })
})
