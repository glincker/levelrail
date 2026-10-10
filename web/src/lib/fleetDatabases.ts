import type { DatabaseListEntry } from '../types/databaseDetail'
import type { DatabaseUsage, VolumeUsageItem } from '../queries/usageSummary'

export type DatabaseHealth = 'healthy' | 'unhealthy' | 'stopped' | 'starting'

export interface DatabaseRow {
  name: string
  engine: string
  version: string
  health: DatabaseHealth
  sizeBytes?: number
  cpuPercent?: number
  memoryBytes?: number
}

export interface DatabaseFleetSummary {
  total: number
  counts: Record<DatabaseHealth, number>
  engines: { engine: string; count: number }[]
  totalBytes: number
  sizedCount: number
  largest?: DatabaseRow
  busiest?: DatabaseRow
  rows: DatabaseRow[]
}

const HEALTH_ORDER: Record<DatabaseHealth, number> = {
  unhealthy: 0,
  starting: 1,
  healthy: 2,
  stopped: 3,
}

export function classifyDatabase(d: DatabaseListEntry): DatabaseHealth {
  if (d.suspended) return 'stopped'
  switch (d.status.variant) {
    case 'success':
      return 'healthy'
    case 'destructive':
      return 'unhealthy'
    default:
      return 'starting'
  }
}

// Size is the sum of the database's own named volumes on the control
// plane node; a database placed elsewhere has no size here, not a zero.
export function summarizeDatabases(
  databases: DatabaseListEntry[],
  usage: DatabaseUsage[],
  volumes: VolumeUsageItem[],
): DatabaseFleetSummary {
  const usageByName = new Map(usage.map((u) => [u.name, u]))
  const sizeByName = new Map<string, number>()
  for (const v of volumes) {
    if (v.owner_kind !== 'database' || !v.owner || v.size_bytes === undefined) {
      continue
    }
    sizeByName.set(v.owner, (sizeByName.get(v.owner) ?? 0) + v.size_bytes)
  }
  const counts: Record<DatabaseHealth, number> = {
    healthy: 0,
    unhealthy: 0,
    stopped: 0,
    starting: 0,
  }
  const engineCounts = new Map<string, number>()
  const rows = databases.map((d): DatabaseRow => {
    const health = classifyDatabase(d)
    counts[health] += 1
    engineCounts.set(d.engine, (engineCounts.get(d.engine) ?? 0) + 1)
    const u = usageByName.get(d.name)
    return {
      name: d.name,
      engine: d.engine,
      version: d.version,
      health,
      sizeBytes: sizeByName.get(d.name),
      cpuPercent: u?.cpu_percent,
      memoryBytes: u?.memory_usage_bytes,
    }
  })
  rows.sort(
    (a, b) =>
      HEALTH_ORDER[a.health] - HEALTH_ORDER[b.health] ||
      a.name.localeCompare(b.name),
  )
  let largest: DatabaseRow | undefined
  let busiest: DatabaseRow | undefined
  let totalBytes = 0
  let sizedCount = 0
  for (const r of rows) {
    if (r.sizeBytes !== undefined) {
      totalBytes += r.sizeBytes
      sizedCount += 1
      if (r.sizeBytes > (largest?.sizeBytes ?? 0)) largest = r
    }
    if ((r.cpuPercent ?? 0) > (busiest?.cpuPercent ?? 0)) busiest = r
  }
  return {
    total: databases.length,
    counts,
    engines: [...engineCounts.entries()]
      .map(([engine, count]) => ({ engine, count }))
      .sort((a, b) => b.count - a.count || a.engine.localeCompare(b.engine)),
    totalBytes,
    sizedCount,
    largest,
    busiest,
    rows,
  }
}
