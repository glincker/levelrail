import { describe, expect, it } from 'vitest'
import type { AttentionStep } from './dashboardHome'
import { causeCount, groupAttention } from './dashboardHome'
import { sortApps } from './dashboardApps'
import type { AppListEntry } from '../types/appDetail'

const step = (
  id: string,
  groupKey?: string,
  tone: AttentionStep['tone'] = 'warning',
): AttentionStep => ({
  id,
  source: 'attention',
  tone,
  title: id,
  detail: 'd',
  groupKey,
  actionLabel: 'Open',
  spec: { kind: 'path', to: `/databases/${id}` },
})

describe('groupAttention', () => {
  it('folds steps with one root cause into a single group at the first position', () => {
    const out = groupAttention([
      step('a', 'copy:ssl'),
      step('x'),
      step('b', 'copy:ssl'),
      step('c', 'copy:ssl', 'danger'),
    ])
    expect(out.map((o) => o.id)).toEqual(['group:copy:ssl', 'x'])
    const group = out[0]
    expect(group?.source).toBe('group')
    if (group?.source === 'group') {
      expect(group.items).toHaveLength(3)
      expect(group.tone).toBe('danger')
    }
  })

  it('leaves a lone grouped step as a plain row', () => {
    expect(
      groupAttention([step('a', 'copy:ssl')]).map((o) => o.source),
    ).toEqual(['attention'])
  })
})

describe('causeCount', () => {
  it('counts items sharing a failure class once', () => {
    expect(
      causeCount([
        { id: '1', copy: { code: 'ssl' } },
        { id: '2', copy: { code: 'ssl' } },
        { id: '3' },
      ]),
    ).toBe(2)
  })
})

describe('sortApps', () => {
  it('lists failing apps before healthy ones', () => {
    const app = (name: string, variant: AppListEntry['status']['variant']) =>
      ({ name, status: { label: name, variant } }) as AppListEntry
    expect(
      sortApps([app('b', 'success'), app('a', 'destructive')]).map(
        (a) => a.name,
      ),
    ).toEqual(['a', 'b'])
  })
})
