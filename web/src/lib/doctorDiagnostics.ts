import type { DoctorCheck } from '../queries/systemDoctor'
import type { DoctorGroup } from './doctorModel'

export type DoctorFilter = 'all' | 'fail' | 'warn'

/** filterGroups keeps only checks matching the chip; empty groups drop out. */
export function filterGroups(
  groups: DoctorGroup[],
  filter: DoctorFilter,
): DoctorGroup[] {
  if (filter === 'all') return groups
  return groups
    .map((g) => ({ ...g, checks: g.checks.filter((c) => c.status === filter) }))
    .filter((g) => g.checks.length > 0)
}

/**
 * buildDiagnostics is the plain text an operator pastes into a bug report.
 * Only code, status and message are included: evidence can hold paths and addresses.
 */
export function buildDiagnostics(
  checks: DoctorCheck[],
  checkedAt: Date | null,
): string {
  const bad = checks.filter((c) => c.status !== 'ok')
  const lines = [
    `Doctor diagnostics${checkedAt ? ` (${checkedAt.toISOString()})` : ''}`,
    `${bad.length} of ${checks.length} checks need attention`,
  ]
  for (const c of bad) {
    lines.push(`[${c.status}] ${c.code}: ${c.message}`)
  }
  return lines.join('\n')
}
