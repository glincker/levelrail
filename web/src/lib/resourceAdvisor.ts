import type {
  AdvisorAction,
  AdvisorFinding,
  AdvisorRef,
  AdvisorSeverity,
} from '../types/resourceAdvisor'
import { formatBytes, formatNanoCpus } from './format'

export const SEVERITY_RANK: Record<AdvisorSeverity, number> = {
  warn: 2,
  suggest: 1,
  info: 0,
}

export const SEVERITY_BADGE: Record<
  AdvisorSeverity,
  'warning' | 'info' | 'muted'
> = {
  warn: 'warning',
  suggest: 'info',
  info: 'muted',
}

export const FINDING_KINDS = [
  'memory_limit_missing',
  'cpu_limit_missing',
  'memory_limit_tight',
  'cpu_limit_tight',
  'memory_over_provisioned',
  'cpu_over_provisioned',
  'memory_growth',
  'idle_app',
  'idle_database',
  'engine_tuning',
  'consolidate_databases',
  'disk_forecast',
  'reclaimable_disk',
  'host_memory_pressure',
  'swap_in_use',
  'top_consumers',
  'capacity_unknown',
  'not_enough_data',
] as const

export type FindingKind = (typeof FINDING_KINDS)[number]

export function isKnownKind(kind: string): kind is FindingKind {
  return (FINDING_KINDS as readonly string[]).includes(kind)
}

export function refKey(ref: AdvisorRef): string {
  return `${ref.kind}:${ref.name}`
}

export function savingOf(f: AdvisorFinding): number {
  return (f.impact.saving_memory_bytes ?? 0) + (f.impact.saving_disk_bytes ?? 0)
}

// Renders a limit or sleep value in the unit its action uses.
export function formatActionValue(
  type: AdvisorAction['type'],
  value: number | undefined,
): string | null {
  switch (type) {
    case 'set_memory_limit':
      return value ? formatBytes(value) : null
    case 'set_cpu_limit':
      return value ? formatNanoCpus(value) : null
    default:
      return null
  }
}

export interface FindingGroup {
  ref: AdvisorRef
  findings: AdvisorFinding[]
  severity: AdvisorSeverity
  saving: number
}

// Groups by resource, most severe group first, then by estimated saving.
export function groupByResource(findings: AdvisorFinding[]): FindingGroup[] {
  const groups = new Map<string, FindingGroup>()
  for (const f of findings) {
    const key = refKey(f.resource)
    let g = groups.get(key)
    if (!g) {
      g = { ref: f.resource, findings: [], severity: 'info', saving: 0 }
      groups.set(key, g)
    }
    g.findings.push(f)
    g.saving += savingOf(f)
    if (SEVERITY_RANK[f.severity] > SEVERITY_RANK[g.severity]) {
      g.severity = f.severity
    }
  }
  return [...groups.values()].sort(
    (a, b) =>
      SEVERITY_RANK[b.severity] - SEVERITY_RANK[a.severity] ||
      b.saving - a.saving ||
      refKey(a.ref).localeCompare(refKey(b.ref)),
  )
}

// Orders by estimated saving, then severity; findings with no saving last.
export function sortBySavings(findings: AdvisorFinding[]): AdvisorFinding[] {
  return [...findings].sort(
    (a, b) =>
      savingOf(b) - savingOf(a) ||
      SEVERITY_RANK[b.severity] - SEVERITY_RANK[a.severity] ||
      a.id.localeCompare(b.id),
  )
}

// Splits out "not enough data yet" notes so they never crowd real advice.
export function splitPending(findings: AdvisorFinding[]): {
  advice: AdvisorFinding[]
  pending: AdvisorFinding[]
} {
  const advice: AdvisorFinding[] = []
  const pending: AdvisorFinding[] = []
  for (const f of findings) {
    ;(f.kind === 'not_enough_data' ? pending : advice).push(f)
  }
  return { advice, pending }
}
