import { describe, expect, it } from 'vitest'
import {
  newTaintRow,
  rowsFromTaints,
  taintsFromRows,
  tolerationsFromRows,
} from './taintRows'

describe('taintsFromRows', () => {
  it('trims, skips blank rows and keeps the effect', () => {
    const r = taintsFromRows([
      newTaintRow(' dedicated ', ' gpu ', 'NoExecute'),
      newTaintRow(),
    ])
    expect(r).toEqual({
      taints: [{ key: 'dedicated', value: 'gpu', effect: 'NoExecute' }],
      error: null,
    })
  })

  it('rejects a row with a value but no key', () => {
    expect(taintsFromRows([newTaintRow('', 'x')]).error).toBe('incomplete')
  })

  it('rejects a repeated key and effect pair only', () => {
    const dup = taintsFromRows([newTaintRow('a'), newTaintRow('a')])
    expect(dup.error).toBe('duplicate')
    const ok = taintsFromRows([
      newTaintRow('a', '', 'NoSchedule'),
      newTaintRow('a', '', 'NoExecute'),
    ])
    expect(ok.error).toBeNull()
  })

  it('round trips saved taints', () => {
    const saved = [{ key: 'k', value: 'v', effect: 'NoSchedule' as const }]
    expect(taintsFromRows(rowsFromTaints(saved)).taints).toEqual(saved)
  })
})

describe('tolerationsFromRows', () => {
  it('drops the value for the Exists operator', () => {
    const r = tolerationsFromRows([
      newTaintRow('k', 'v', 'NoSchedule', 'Exists'),
    ])
    expect(r.tolerations).toEqual([
      { key: 'k', operator: 'Exists', value: '', effect: 'NoSchedule' },
    ])
  })
})
