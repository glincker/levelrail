import { describe, expect, it } from 'vitest'
import {
  buildConsumers,
  buildHostGauges,
  buildMemoryBreakdown,
  rankConsumers,
} from './fleetResources'
import type { FleetResourceUsage } from '../types/fleetResourceUsage'

const GIB = 1024 ** 3

describe('consumers', () => {
  const consumers = buildConsumers(
    [{ name: 'web', memory_usage_bytes: 200 * 1024 ** 2, cpu_percent: 3 }],
    [
      { name: 'pg', memory_usage_bytes: 600 * 1024 ** 2, cpu_percent: 1 },
      { name: 'redis', memory_usage_bytes: 50 * 1024 ** 2, cpu_percent: 9 },
      { name: 'idle' },
    ],
  )

  it('ranks apps and databases together by memory', () => {
    const ranked = rankConsumers(consumers, 'memory')
    expect(ranked.map((c) => `${c.kind}:${c.name}`)).toEqual([
      'database:pg',
      'app:web',
      'database:redis',
    ])
  })

  it('ranks by cpu and drops rows with no reading', () => {
    const ranked = rankConsumers(consumers, 'cpu')
    expect(ranked.map((c) => c.name)).toEqual(['redis', 'web', 'pg'])
  })
})

describe('buildMemoryBreakdown', () => {
  const fleet: FleetResourceUsage = {
    nodes: [
      {
        node_id: 'n',
        name: 'host',
        is_local: true,
        memory_total_bytes: 8 * GIB,
        memory_available_bytes: 4 * GIB,
      },
    ],
    fleet: {
      node_count: 1,
      nodes_with_memory_capacity: 1,
      nodes_with_disk_capacity: 0,
    },
  }

  it('splits used memory into apps, databases and system', () => {
    const b = buildMemoryBreakdown(
      fleet,
      buildConsumers(
        [{ name: 'web', memory_usage_bytes: 1 * GIB }],
        [{ name: 'pg', memory_usage_bytes: 2 * GIB }],
      ),
    )
    expect(b).toMatchObject({
      apps: 1 * GIB,
      databases: 2 * GIB,
      system: 1 * GIB,
      free: 4 * GIB,
      systemKnown: true,
    })
  })

  it('is undefined without host capacity', () => {
    expect(buildMemoryBreakdown({ ...fleet, nodes: [] }, [])).toBeUndefined()
  })
})

describe('buildHostGauges', () => {
  it('lists the local node first and derives memory from available bytes', () => {
    const gauges = buildHostGauges(
      [
        { node_id: 'r', name: 'remote', is_local: false, cpu_percent: 10 },
        {
          node_id: 'l',
          name: 'local',
          is_local: true,
          memory_total_bytes: 4 * GIB,
          memory_available_bytes: 1 * GIB,
          disk_total_bytes: 100,
          disk_used_bytes: 40,
          cpu_percent: 50,
        },
      ],
      4,
    )
    expect(gauges[0]?.name).toBe('local')
    expect(gauges[0]?.memory).toEqual({
      usedBytes: 3 * GIB,
      totalBytes: 4 * GIB,
    })
    expect(gauges[0]?.cpu?.cores).toBe(4)
    expect(gauges[1]?.cpu?.cores).toBeUndefined()
  })
})
