import type { ServiceResources } from '../types/appDetail'

const NANO_CPUS_PER_CORE = 1_000_000_000
const PERCENT_PER_CORE = 100

function envRatio(raw: string | undefined, fallback: number): number {
  const n = Number(raw)
  return raw && Number.isFinite(n) && n > 0 && n < 1 ? n : fallback
}

// Used below this share of what is reserved reads as "reserved but idle".
export const IDLE_RATIO = envRatio(
  import.meta.env.VITE_RESERVED_IDLE_RATIO as string | undefined,
  0.2,
)

export type ResourceKind = 'app' | 'database'

export interface Reservable {
  kind: ResourceKind
  name: string
  projectId: string
  environment: string
  replicas: number
  suspended: boolean
  resources?: ServiceResources | null
}

export interface UsageReading {
  cpuPercent?: number
  memoryBytes?: number
}

export interface Capacity {
  cpuCores?: number
  memoryBytes?: number
  // True when every node reported that figure, so over-commit is checkable.
  cpuComplete: boolean
  memoryComplete: boolean
}

export type Verdict =
  'overCommitted' | 'idle' | 'headroom' | 'noLimits' | 'unknownCapacity'

export interface ResourceTotals {
  reserved: number
  used: number | undefined
  capacity: number | undefined
  capacityComplete: boolean
  overBy: number
  unsetCount: number
  verdict: Verdict
}

export interface Reservation {
  key: string
  kind: ResourceKind
  name: string
  cpuCores: number
  memoryBytes: number
  usedCpuCores?: number
  usedMemoryBytes?: number
}

export interface GroupRow {
  key: string
  cpuCores: number
  memoryBytes: number
  usedCpuCores: number
  usedMemoryBytes: number
  count: number
  unsetCount: number
}

export interface ReservedSummary {
  cpu: ResourceTotals
  memory: ResourceTotals
  reservations: Reservation[]
  active: number
  suspended: number
}

export function usageKey(kind: ResourceKind, name: string): string {
  return `${kind}:${name}`
}

function replicaCount(r: Reservable): number {
  return r.kind === 'app' ? Math.max(1, r.replicas) : 1
}

function cpuOf(r: Reservable): number {
  const nano = r.resources?.nano_cpus ?? 0
  return nano > 0 ? (nano / NANO_CPUS_PER_CORE) * replicaCount(r) : 0
}

function memoryOf(r: Reservable): number {
  const bytes = r.resources?.memory_bytes ?? 0
  return bytes > 0 ? bytes * replicaCount(r) : 0
}

export function cpuCoresFromPercent(percent: number): number {
  return percent / PERCENT_PER_CORE
}

function verdictFor(
  reserved: number,
  used: number | undefined,
  capacity: number | undefined,
  complete: boolean,
): { verdict: Verdict; overBy: number } {
  if (reserved <= 0) return { verdict: 'noLimits', overBy: 0 }
  if (capacity !== undefined && complete && reserved > capacity) {
    return { verdict: 'overCommitted', overBy: reserved - capacity }
  }
  if (used !== undefined && used / reserved < IDLE_RATIO) {
    return { verdict: 'idle', overBy: 0 }
  }
  if (capacity === undefined || !complete) {
    return { verdict: 'unknownCapacity', overBy: 0 }
  }
  return { verdict: 'headroom', overBy: 0 }
}

// Suspended resources hold no container, so they reserve nothing and are
// counted apart. A resource with no limit is never guessed at: it adds
// zero to reserved and one to unsetCount.
export function summarizeReserved(
  resources: Reservable[],
  usage: ReadonlyMap<string, UsageReading>,
  capacity: Capacity,
  usageKnown: boolean,
): ReservedSummary {
  const reservations: Reservation[] = []
  let cpuReserved = 0
  let memReserved = 0
  let cpuUsed = 0
  let memUsed = 0
  let cpuUnset = 0
  let memUnset = 0
  let suspended = 0
  for (const r of resources) {
    if (r.suspended) {
      suspended += 1
      continue
    }
    const key = usageKey(r.kind, r.name)
    const cpu = cpuOf(r)
    const mem = memoryOf(r)
    if (cpu === 0) cpuUnset += 1
    if (mem === 0) memUnset += 1
    cpuReserved += cpu
    memReserved += mem
    const reading = usage.get(key)
    const usedCpu =
      reading?.cpuPercent === undefined
        ? undefined
        : cpuCoresFromPercent(reading.cpuPercent)
    cpuUsed += usedCpu ?? 0
    memUsed += reading?.memoryBytes ?? 0
    reservations.push({
      key,
      kind: r.kind,
      name: r.name,
      cpuCores: cpu,
      memoryBytes: mem,
      usedCpuCores: usedCpu,
      usedMemoryBytes: reading?.memoryBytes,
    })
  }
  const cpuV = verdictFor(
    cpuReserved,
    usageKnown ? cpuUsed : undefined,
    capacity.cpuCores,
    capacity.cpuComplete,
  )
  const memV = verdictFor(
    memReserved,
    usageKnown ? memUsed : undefined,
    capacity.memoryBytes,
    capacity.memoryComplete,
  )
  return {
    cpu: {
      reserved: cpuReserved,
      used: usageKnown ? cpuUsed : undefined,
      capacity: capacity.cpuCores,
      capacityComplete: capacity.cpuComplete,
      overBy: cpuV.overBy,
      unsetCount: cpuUnset,
      verdict: cpuV.verdict,
    },
    memory: {
      reserved: memReserved,
      used: usageKnown ? memUsed : undefined,
      capacity: capacity.memoryBytes,
      capacityComplete: capacity.memoryComplete,
      overBy: memV.overBy,
      unsetCount: memUnset,
      verdict: memV.verdict,
    },
    reservations,
    active: reservations.length,
    suspended,
  }
}

export type GroupBy = 'project' | 'environment'

export function groupReservations(
  resources: Reservable[],
  summary: ReservedSummary,
  by: GroupBy,
  labelFor: (r: Reservable) => string,
): GroupRow[] {
  const byKey = new Map(summary.reservations.map((r) => [r.key, r]))
  const rows = new Map<string, GroupRow>()
  for (const r of resources) {
    const res = byKey.get(usageKey(r.kind, r.name))
    if (!res) continue
    const label = by === 'project' ? labelFor(r) : r.environment || labelFor(r)
    const row = rows.get(label) ?? {
      key: label,
      cpuCores: 0,
      memoryBytes: 0,
      usedCpuCores: 0,
      usedMemoryBytes: 0,
      count: 0,
      unsetCount: 0,
    }
    row.cpuCores += res.cpuCores
    row.memoryBytes += res.memoryBytes
    row.usedCpuCores += res.usedCpuCores ?? 0
    row.usedMemoryBytes += res.usedMemoryBytes ?? 0
    row.count += 1
    if (res.cpuCores === 0 && res.memoryBytes === 0) row.unsetCount += 1
    rows.set(label, row)
  }
  return [...rows.values()].sort((a, b) => b.memoryBytes - a.memoryBytes)
}

export function topReservers(
  summary: ReservedSummary,
  limit: number,
): Reservation[] {
  return [...summary.reservations]
    .filter((r) => r.memoryBytes > 0 || r.cpuCores > 0)
    .sort((a, b) => b.memoryBytes - a.memoryBytes || b.cpuCores - a.cpuCores)
    .slice(0, limit)
}
