import { describe, expect, it } from 'vitest'
import type { FwRule } from '../../queries/firewallEdge'
import { filterRules, selectState, sortRules } from './firewallListModel'

const rule = (id: string, name: string, hits: number, at: string): FwRule => ({
  id,
  name,
  enabled: true,
  action: 'deny',
  position: 1,
  conditions: [{ field: 'ip', op: 'eq', value: `10.0.0.${id}` }],
  updated_at: at,
  hits: { total: hits, sparkline: [] },
})

const rules = [
  rule('1', 'beta', 5, '2026-01-01'),
  rule('2', 'alpha', 9, '2026-03-01'),
  rule('3', 'gamma', 1, '2026-02-01'),
]

describe('firewall list model', () => {
  it('filters by every word across name and conditions', () => {
    expect(filterRules(rules, 'alp').map((r) => r.id)).toEqual(['2'])
    expect(filterRules(rules, '10.0.0.3 gam').map((r) => r.id)).toEqual(['3'])
    expect(filterRules(rules, '  ')).toBe(rules)
  })

  it('sorts without mutating the input', () => {
    expect(sortRules(rules, 'name').map((r) => r.id)).toEqual(['2', '1', '3'])
    expect(sortRules(rules, 'hits').map((r) => r.id)).toEqual(['2', '1', '3'])
    expect(sortRules(rules, 'recent').map((r) => r.id)).toEqual(['2', '3', '1'])
    expect(rules.map((r) => r.id)).toEqual(['1', '2', '3'])
  })

  it('reports the select-all state', () => {
    expect(selectState(rules, new Set())).toBe('none')
    expect(selectState(rules, new Set(['1']))).toBe('some')
    expect(selectState(rules, new Set(['1', '2', '3']))).toBe('all')
  })
})
