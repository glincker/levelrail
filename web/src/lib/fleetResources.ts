import type { AppMetricsSummary } from '../types/appsMetrics'
import type { AppResourceUsage } from '../types/appResourceUsage'
import type {
  FleetResourceUsage,
  NodeResourceUsage,
} from '../types/fleetResourceUsage'
import type { DatabaseUsage } from '../queries/usageSummary'

export type ConsumerKind = 'app' | 'database'
export type ConsumerSort = 'memory' | 'cpu'

export interface Consumer {
  kind: ConsumerKind
  name: string
  memoryBytes: number
  cpuPercent: number
  // Request-rate trend for an app; databases have none.
  spark?: number[]
}

export function buildConsumers(
  apps: AppResourceUsage[],
  databases: DatabaseUsage[],
  metrics: AppMetricsSummary[] = [],
): Consumer[] {
  const sparkByApp = new Map(metrics.map((m) => [m.name, m.spark]))
  const out: Consumer[] = []
  for (const a of apps) {
    out.push({
      kind: 'app',
      name: a.name,
      memoryBytes: a.memory_usage_bytes ?? 0,
      cpuPercent: a.cpu_percent ?? 0,
      spark: sparkByApp.get(a.name),
    })
  }
  for (const d of databases) {
    out.push({
      kind: 'database',
      name: d.name,
      memoryBytes: d.memory_usage_bytes ?? 0,
      cpuPercent: d.cpu_percent ?? 0,
    })
  }
  return out
}

export function rankConsumers(
  consumers: Consumer[],
  sort: ConsumerSort,
): Consumer[] {
  const key = (c: Consumer) =>
    sort === 'memory' ? c.memoryBytes : c.cpuPercent
  return [...consumers]
    .filter((c) => c.memoryBytes > 0 || c.cpuPercent > 0)
    .sort((a, b) => key(b) - key(a) || a.name.localeCompare(b.name))
}

export interface MemoryBreakdown {
  totalBytes: number
  apps: number
  databases: number
  system: number
  free: number
  // False when the host did not report available memory: system and free
  // are then folded into one remainder shown as free.
  systemKnown: boolean
}

// Scoped to the nodes that report host capacity, because that is the only
// denominator the server has.
export function buildMemoryBreakdown(
  fleet: FleetResourceUsage | undefined,
  consumers: Consumer[],
): MemoryBreakdown | undefined {
  const capacity = (fleet?.nodes ?? []).filter(
    (n) => (n.memory_total_bytes ?? 0) > 0,
  )
  if (capacity.length === 0) return undefined
  const totalBytes = capacity.reduce(
    (s, n) => s + (n.memory_total_bytes ?? 0),
    0,
  )
  const known = capacity.every((n) => n.memory_available_bytes !== undefined)
  const availableBytes = capacity.reduce(
    (s, n) => s + (n.memory_available_bytes ?? 0),
    0,
  )
  const sum = (kind: ConsumerKind) =>
    consumers
      .filter((c) => c.kind === kind)
      .reduce((s, c) => s + c.memoryBytes, 0)
  const apps = Math.min(sum('app'), totalBytes)
  const databases = Math.min(sum('database'), Math.max(0, totalBytes - apps))
  const used = known ? totalBytes - availableBytes : apps + databases
  const system = known ? Math.max(0, used - apps - databases) : 0
  const free = Math.max(0, totalBytes - apps - databases - system)
  return { totalBytes, apps, databases, system, free, systemKnown: known }
}

export interface HostGauge {
  id: string
  name: string
  isLocal: boolean
  memory?: { usedBytes: number; totalBytes: number }
  disk?: { usedBytes: number; totalBytes: number }
  // Sum of container CPU in percent of one core, with the host's core
  // count when the server knows it.
  cpu?: { percent: number; cores?: number }
}

export function buildHostGauges(
  nodes: NodeResourceUsage[],
  localCores: number | undefined,
): HostGauge[] {
  return [...nodes]
    .sort((a, b) => Number(b.is_local) - Number(a.is_local))
    .map((n) => {
      const memTotal = n.memory_total_bytes
      const memUsed =
        n.memory_available_bytes !== undefined && memTotal !== undefined
          ? memTotal - n.memory_available_bytes
          : n.memory_usage_bytes
      return {
        id: n.node_id,
        name: n.name,
        isLocal: n.is_local,
        memory:
          memTotal && memUsed !== undefined
            ? { usedBytes: memUsed, totalBytes: memTotal }
            : undefined,
        disk:
          n.disk_total_bytes && n.disk_used_bytes !== undefined
            ? { usedBytes: n.disk_used_bytes, totalBytes: n.disk_total_bytes }
            : undefined,
        cpu:
          n.cpu_percent !== undefined
            ? {
                percent: n.cpu_percent,
                cores: n.is_local && localCores ? localCores : undefined,
              }
            : undefined,
      }
    })
}

export function ratio(used: number, total: number): number {
  return total > 0 ? Math.min(1, Math.max(0, used / total)) : 0
}
