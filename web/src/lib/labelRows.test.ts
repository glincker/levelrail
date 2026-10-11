import { describe, expect, it } from 'vitest'
import {
  labelsFromRows,
  newLabelRow,
  rowsFromLabels,
  sameLabels,
} from './labelRows'

describe('labelsFromRows', () => {
  it('builds a map and skips blank rows', () => {
    const result = labelsFromRows([
      newLabelRow('disk', 'ssd'),
      newLabelRow(),
      newLabelRow(' zone ', ' eu '),
    ])
    expect(result).toEqual({
      labels: { disk: 'ssd', zone: 'eu' },
      error: null,
    })
  })

  it('flags a half-filled row', () => {
    expect(labelsFromRows([newLabelRow('disk', '')]).error).toBe('incomplete')
    expect(labelsFromRows([newLabelRow('', 'ssd')]).error).toBe('incomplete')
  })

  it('flags a repeated key', () => {
    const rows = [newLabelRow('disk', 'ssd'), newLabelRow('disk', 'hdd')]
    expect(labelsFromRows(rows).error).toBe('duplicate')
  })
})

describe('rowsFromLabels and sameLabels', () => {
  it('round trips sorted by key', () => {
    const rows = rowsFromLabels({ b: '2', a: '1' })
    expect(rows.map((r) => r.key)).toEqual(['a', 'b'])
    expect(labelsFromRows(rows).labels).toEqual({ a: '1', b: '2' })
  })

  it('compares sets regardless of order', () => {
    expect(sameLabels({ a: '1', b: '2' }, { b: '2', a: '1' })).toBe(true)
    expect(sameLabels({ a: '1' }, { a: '2' })).toBe(false)
    expect(sameLabels({ a: '1' }, {})).toBe(false)
  })
})
