import { describe, expect, it } from 'vitest'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { summarizeDatabases } from './fleetDatabases'

function entry(
  name: string,
  variant: 'success' | 'destructive' | 'muted',
  over: Partial<DatabaseListEntry> = {},
): DatabaseListEntry {
  return {
    name,
    engine: 'postgres',
    version: '16',
    status: { label: variant, variant },
    ...over,
  }
}

describe('summarizeDatabases', () => {
  const dbs = [
    entry('a', 'success'),
    entry('b', 'destructive', { engine: 'redis' }),
    entry('c', 'success', { suspended: true }),
    entry('d', 'muted'),
  ]
  const s = summarizeDatabases(
    dbs,
    [
      { name: 'a', cpu_percent: 5 },
      { name: 'b', cpu_percent: 40 },
    ],
    [
      {
        name: 'db-a-data',
        owner_kind: 'database',
        owner: 'a',
        size_bytes: 100,
      },
      {
        name: 'db-a-certs',
        owner_kind: 'database',
        owner: 'a',
        size_bytes: 20,
      },
      { name: 'db-b-data', owner_kind: 'database', owner: 'b', size_bytes: 50 },
      { name: 'app-x', owner_kind: 'app', owner: 'x', size_bytes: 9999 },
    ],
  )

  it('classifies health with stopped taking precedence', () => {
    expect(s.counts).toEqual({
      healthy: 1,
      unhealthy: 1,
      stopped: 1,
      starting: 1,
    })
    expect(s.rows[0]?.name).toBe('b')
  })

  it('sums database volumes only and picks largest and busiest', () => {
    expect(s.totalBytes).toBe(170)
    expect(s.sizedCount).toBe(2)
    expect(s.largest?.name).toBe('a')
    expect(s.busiest?.name).toBe('b')
  })

  it('counts engines', () => {
    expect(s.engines).toEqual([
      { engine: 'postgres', count: 3 },
      { engine: 'redis', count: 1 },
    ])
  })

  it('leaves size undefined for databases with no measured volume', () => {
    expect(s.rows.find((r) => r.name === 'c')?.sizeBytes).toBeUndefined()
  })
})
