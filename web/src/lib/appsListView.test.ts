import { afterEach, describe, expect, it } from 'vitest'
import type { AppListEntry } from '../types/appDetail'
import {
  countBuckets,
  filterApps,
  loadViewMode,
  parseStatusSearch,
  statusBucket,
  storeViewMode,
} from './appsListView'
import { EMPTY_FILTERS } from './appViews'

function app(
  name: string,
  variant: 'success' | 'destructive' | 'muted',
  label: string,
): AppListEntry {
  return {
    name,
    image: `${name}:1`,
    status: { variant, label },
  } as AppListEntry
}

const APPS = [
  app('a', 'success', 'Healthy'),
  app('b', 'success', 'Healthy'),
  app('c', 'destructive', 'Attention needed'),
  app('d', 'muted', 'Reconciling'),
  app('e', 'muted', 'Stopped'),
]

afterEach(() => {
  window.localStorage.clear()
})

describe('status buckets', () => {
  it('classifies each summary', () => {
    expect(APPS.map(statusBucket)).toEqual([
      'running',
      'running',
      'failing',
      'deploying',
      'stopped',
    ])
  })

  it('counts every bucket', () => {
    expect(countBuckets(APPS)).toEqual({
      running: 2,
      deploying: 1,
      failing: 1,
      stopped: 1,
    })
  })

  it('filters by bucket and by query together', () => {
    expect(
      filterApps(APPS, EMPTY_FILTERS, 'running').map((a) => a.name),
    ).toEqual(['a', 'b'])
    expect(
      filterApps(APPS, { ...EMPTY_FILTERS, query: 'b' }, 'running').map(
        (a) => a.name,
      ),
    ).toEqual(['b'])
    expect(filterApps(APPS, EMPTY_FILTERS, null)).toHaveLength(5)
  })

  it('accepts only known status search values', () => {
    expect(parseStatusSearch('failing')).toBe('failing')
    expect(parseStatusSearch('nope')).toBeNull()
    expect(parseStatusSearch(3)).toBeNull()
  })
})

describe('view mode persistence', () => {
  it('defaults to table and remembers grid', () => {
    expect(loadViewMode()).toBe('table')
    storeViewMode('grid')
    expect(loadViewMode()).toBe('grid')
    storeViewMode('table')
    expect(loadViewMode()).toBe('table')
  })
})
