import { describe, expect, it } from 'vitest'
import type { DatabaseListEntry } from '../types/databaseDetail'
import {
  EMPTY_DB_FILTERS,
  countByStatus,
  databaseViewFilter,
  filterDatabases,
  filtersFromSearch,
  parseDatabasesSearch,
  searchFromFilters,
  tagOptions,
} from './databasesFilter'

function db(over: Partial<DatabaseListEntry>): DatabaseListEntry {
  return {
    name: 'db',
    engine: 'postgres',
    version: '16',
    status: { label: 'Healthy', variant: 'success' },
    ...over,
  }
}

const rows = [
  db({ name: 'orders', engine: 'postgres' }),
  db({ name: 'cache', engine: 'redis' }),
  db({
    name: 'legacy',
    engine: 'mysql',
    suspended: true,
    status: { label: 'Stopped', variant: 'muted' },
  }),
  db({
    name: 'events',
    engine: 'postgres',
    status: { label: 'Attention needed', variant: 'destructive' },
  }),
]

describe('filterDatabases', () => {
  it('narrows rows by engine and status', () => {
    const byEngine = filterDatabases(rows, {
      ...EMPTY_DB_FILTERS,
      engines: ['postgres'],
    })
    expect(byEngine.map((r) => r.name)).toEqual(['orders', 'events'])
    const both = filterDatabases(rows, {
      ...EMPTY_DB_FILTERS,
      engines: ['postgres'],
      status: 'attention',
    })
    expect(both.map((r) => r.name)).toEqual(['events'])
    const stopped = filterDatabases(rows, {
      ...EMPTY_DB_FILTERS,
      status: 'stopped',
    })
    expect(stopped.map((r) => r.name)).toEqual(['legacy'])
    expect(
      filterDatabases(rows, { ...EMPTY_DB_FILTERS, q: 'CACH' }).map(
        (r) => r.name,
      ),
    ).toEqual(['cache'])
  })

  it('keeps only rows carrying every selected tag', () => {
    const tagged = [
      db({ name: 'a', tags: ['prod', 'team:billing'] }),
      db({ name: 'b', tags: ['prod'] }),
      db({ name: 'c' }),
    ]
    expect(
      filterDatabases(tagged, { ...EMPTY_DB_FILTERS, tags: ['prod'] }).map(
        (r) => r.name,
      ),
    ).toEqual(['a', 'b'])
    expect(
      filterDatabases(tagged, {
        ...EMPTY_DB_FILTERS,
        tags: ['prod', 'team:billing'],
      }).map((r) => r.name),
    ).toEqual(['a'])
    expect(tagOptions(tagged)).toEqual(['prod', 'team:billing'])
  })

  it('counts every row into one status bucket', () => {
    expect(countByStatus(rows)).toEqual({
      healthy: 2,
      attention: 1,
      reconciling: 0,
      stopped: 1,
    })
  })
})

describe('URL state', () => {
  it('round trips filters through the search params', () => {
    const filters = {
      q: 'ord',
      engines: ['postgres', 'redis'],
      status: 'healthy' as const,
      tags: ['team:billing'],
    }
    const search = searchFromFilters(filters)
    expect(search).toEqual({
      q: 'ord',
      engine: 'postgres,redis',
      status: 'healthy',
      tag: 'team:billing',
    })
    expect(
      filtersFromSearch(parseDatabasesSearch({ ...search, create: '1' })),
    ).toEqual(filters)
    expect(searchFromFilters(EMPTY_DB_FILTERS)).toEqual({})
  })

  it('drops unknown status and empty engines', () => {
    expect(parseDatabasesSearch({ status: 'nope', engine: ' , ' })).toEqual({})
  })
})

describe('databaseViewFilter', () => {
  it('stores the tag filter and restores it through the URL parser', () => {
    const filters = {
      ...EMPTY_DB_FILTERS,
      q: 'pg',
      engines: ['postgres'],
      tags: ['prod', 'billing'],
    }
    const view = databaseViewFilter(filters, 'tier=db')
    expect(view.tag).toBe('prod,billing')
    expect(filtersFromSearch(parseDatabasesSearch(view))).toEqual(filters)
  })
})
