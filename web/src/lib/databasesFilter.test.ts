import { describe, expect, it } from 'vitest'
import type { DatabaseListEntry } from '../types/databaseDetail'
import {
  EMPTY_DB_FILTERS,
  countByStatus,
  filterDatabases,
  filtersFromSearch,
  parseDatabasesSearch,
  runLimited,
  searchFromFilters,
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
    }
    const search = searchFromFilters(filters)
    expect(search).toEqual({
      q: 'ord',
      engine: 'postgres,redis',
      status: 'healthy',
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

describe('runLimited', () => {
  it('never exceeds the limit and keeps order', async () => {
    let inFlight = 0
    let peak = 0
    const out = await runLimited([1, 2, 3, 4, 5], 2, async (n) => {
      inFlight += 1
      peak = Math.max(peak, inFlight)
      await Promise.resolve()
      inFlight -= 1
      return n * 2
    })
    expect(out).toEqual([2, 4, 6, 8, 10])
    expect(peak).toBe(2)
  })
})
