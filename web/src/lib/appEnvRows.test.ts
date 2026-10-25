import { describe, expect, it } from 'vitest'
import { toRows } from './appEnvRows'

describe('toRows', () => {
  it('labels added, removed and changed keys in key order', () => {
    const rows = toRows({
      onlyA: [{ key: 'B', secret: false, a: '1' }],
      onlyB: [{ key: 'C', secret: false, b: '2' }],
      differ: [{ key: 'A', secret: false, a: 'x', b: 'y' }],
      same: 0,
    })
    expect(rows.map((r) => [r.key, r.status])).toEqual([
      ['A', 'changed'],
      ['B', 'added'],
      ['C', 'removed'],
    ])
  })

  it('never carries a value for secret keys', () => {
    const rows = toRows({
      onlyA: [{ key: 'TOKEN', secret: true }],
      onlyB: [],
      differ: [{ key: 'KEY', secret: true }],
      same: 0,
    })
    expect(rows.every((r) => r.secret && r.a === undefined)).toBe(true)
  })
})
