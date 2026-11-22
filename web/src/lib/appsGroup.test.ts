import { describe, expect, it } from 'vitest'
import type { AppListEntry } from '../types/appDetail'
import { groupApps } from './appsGroup'

const app = (name: string, extra: Partial<AppListEntry>): AppListEntry =>
  ({ name, ...extra }) as AppListEntry

describe('groupApps', () => {
  it('groups by node A to Z with the ungrouped bucket last', () => {
    const groups = groupApps(
      [
        app('a', {}),
        app('b', { node_name: 'zeta' }),
        app('c', { node_name: 'alpha' }),
        app('d', { node_name: 'zeta' }),
      ],
      'node',
    )
    expect(groups.map((g) => [g.label, g.apps.map((x) => x.name)])).toEqual([
      ['alpha', ['c']],
      ['zeta', ['b', 'd']],
      ['', ['a']],
    ])
  })

  it('resolves project names from ids', () => {
    const groups = groupApps(
      [app('a', { project_id: 'p1' })],
      'project',
      new Map([['p1', 'Shop']]),
    )
    expect(groups[0]?.label).toBe('Shop')
  })
})
