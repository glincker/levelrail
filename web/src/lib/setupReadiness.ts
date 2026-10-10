// Pure grouping and scoring for the setup wizard's readiness view.

import type { DoctorCheck, DoctorReport } from '../queries/systemDoctor'
import { hardFailures } from './setupWizard'

export const READINESS_CATEGORIES = [
  'runtime',
  'network',
  'security',
  'storage',
  'capacity',
] as const
export type ReadinessCategoryId = (typeof READINESS_CATEGORIES)[number]
export type CategoryVerdict = 'ready' | 'attention' | 'blocked'

const CODE_PREFIXES: ReadonlyArray<[string, ReadinessCategoryId]> = [
  ['port_', 'network'],
  ['external_reachability_', 'network'],
  ['registry_reachability_', 'network'],
  ['cross_node_ingress', 'network'],
  ['nas_', 'storage'],
]

const CODE_EXACT: Readonly<Record<string, ReadinessCategoryId>> = {
  docker: 'runtime',
  database: 'runtime',
  data_dir_writable: 'runtime',
  clock_skew: 'runtime',
  public_ip: 'network',
  firewall: 'network',
  ingress_edge: 'network',
  agent_advertise_reachability: 'network',
  master_key_rotation: 'security',
  secret_binding: 'security',
  stale_secrets: 'security',
  container_hardening: 'security',
  disk_space: 'storage',
  disk_io_latency: 'storage',
  control_plane_backup: 'storage',
  control_plane_dr: 'storage',
  ram: 'capacity',
  cpu: 'capacity',
}

/** categoryOf maps a doctor check code to its readiness category; unknown codes land in runtime so a new check is never hidden. */
export function categoryOf(code: string): ReadinessCategoryId {
  const exact = CODE_EXACT[code]
  if (exact) return exact
  const prefixed = CODE_PREFIXES.find(([p]) => code.startsWith(p))
  return prefixed ? prefixed[1] : 'runtime'
}

export interface CategoryReadiness {
  id: ReadinessCategoryId
  checks: DoctorCheck[]
  ok: number
  warn: number
  fail: number
  unknown: number
  verdict: CategoryVerdict
}

export interface Readiness {
  categories: CategoryReadiness[]
  /** Share of decided checks that passed, 0 to 100; null when nothing could be decided. */
  score: number | null
  verdict: CategoryVerdict
  total: number
  passed: number
  warnings: DoctorCheck[]
  blocking: DoctorCheck[]
}

/** buildReadiness groups the doctor bundle by category and derives a score and verdicts; unknown checks are counted but excluded from the score. */
export function buildReadiness(report: DoctorReport): Readiness {
  const blocking = hardFailures(report)
  const blockingCodes = new Set(blocking.map((c) => c.code))

  const categories = READINESS_CATEGORIES.map((id): CategoryReadiness => {
    const checks = report.checks.filter((c) => categoryOf(c.code) === id)
    const count = (s: DoctorCheck['status']) =>
      checks.filter((c) => c.status === s).length
    const fail = count('fail')
    const warn = count('warn')
    const hasBlocking = checks.some((c) => blockingCodes.has(c.code))
    let verdict: CategoryVerdict = 'ready'
    if (hasBlocking) verdict = 'blocked'
    else if (fail + warn > 0) verdict = 'attention'
    return {
      id,
      checks,
      ok: count('ok'),
      warn,
      fail,
      unknown: count('unknown'),
      verdict,
    }
  }).filter((c) => c.checks.length > 0)

  const ok = report.checks.filter((c) => c.status === 'ok').length
  const decided = report.checks.filter((c) => c.status !== 'unknown').length
  const warnings = report.checks.filter(
    (c) => c.status !== 'ok' && !blockingCodes.has(c.code),
  )
  let verdict: CategoryVerdict = 'ready'
  if (blocking.length > 0) verdict = 'blocked'
  else if (warnings.some((c) => c.status !== 'unknown')) verdict = 'attention'

  return {
    categories,
    score: decided === 0 ? null : Math.round((ok / decided) * 100),
    verdict,
    total: report.checks.length,
    passed: ok,
    warnings,
    blocking,
  }
}

export interface Capacity {
  cpuCores?: number
  ramBytes?: number
  diskFreeBytes?: number
  diskWriteMs?: number
}

const UNIT_BYTES: Readonly<Record<string, number>> = {
  B: 1,
  KiB: 1024,
  MiB: 1024 ** 2,
  GiB: 1024 ** 3,
  TiB: 1024 ** 4,
}

function checkByCode(report: DoctorReport, code: string) {
  return report.checks.find((c) => c.code === code && c.status !== 'unknown')
}

/** parseCapacity reads real numbers out of the resource checks' messages; any value that cannot be parsed stays undefined rather than guessed. */
export function parseCapacity(report: DoctorReport): Capacity {
  const out: Capacity = {}
  const cpu = checkByCode(report, 'cpu')?.message.match(/^(\d+) core/)
  if (cpu?.[1]) out.cpuCores = Number(cpu[1])
  const ram = checkByCode(report, 'ram')?.message.match(/^(\d+) bytes total/)
  if (ram?.[1]) out.ramBytes = Number(ram[1])
  const disk = checkByCode(report, 'disk_space')?.message.match(
    /^([\d.]+) (B|KiB|MiB|GiB|TiB) free/,
  )
  if (disk?.[1] && disk[2]) {
    out.diskFreeBytes = Number(disk[1]) * (UNIT_BYTES[disk[2]] ?? 1)
  }
  const lat = checkByCode(report, 'disk_io_latency')?.message.match(
    /in ([\d.]+)(µs|ms|s)\b/,
  )
  if (lat?.[1] && lat[2]) {
    const factor = lat[2] === 's' ? 1000 : lat[2] === 'ms' ? 1 : 0.001
    out.diskWriteMs = Number(lat[1]) * factor
  }
  return out
}

// Planning assumptions behind the app estimate, shown to the operator next to the number.
export const ESTIMATE_RESERVED_BYTES = 1024 ** 3
export const ESTIMATE_APP_BYTES = 256 * 1024 ** 2
export const ESTIMATE_DISK_RESERVED_BYTES = 5 * 1024 ** 3
export const ESTIMATE_APP_DISK_BYTES = 1024 ** 3

export interface AppEstimate {
  count: number
  limitedBy: 'memory' | 'disk'
  usedDisk: boolean
}

/** estimateSmallApps returns how many small apps fit given memory and, when known, free disk; null when memory is unknown or too small to say. */
export function estimateSmallApps(capacity: Capacity): AppEstimate | null {
  if (capacity.ramBytes === undefined) return null
  const byMemory = Math.floor(
    (capacity.ramBytes - ESTIMATE_RESERVED_BYTES) / ESTIMATE_APP_BYTES,
  )
  if (byMemory < 1) return null
  if (capacity.diskFreeBytes === undefined) {
    return { count: byMemory, limitedBy: 'memory', usedDisk: false }
  }
  const byDisk = Math.max(
    0,
    Math.floor(
      (capacity.diskFreeBytes - ESTIMATE_DISK_RESERVED_BYTES) /
        ESTIMATE_APP_DISK_BYTES,
    ),
  )
  return byDisk < byMemory
    ? { count: byDisk, limitedBy: 'disk', usedDisk: true }
    : { count: byMemory, limitedBy: 'memory', usedDisk: true }
}
