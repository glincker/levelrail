import type {
  PipelineOverviewFilters,
  PipelineRunRow,
} from '../types/pipelineOverview'

const DAY_MS = 24 * 60 * 60 * 1000

export function hasActiveFilters(filters: PipelineOverviewFilters): boolean {
  return Object.values(filters).some(Boolean)
}

export function formatSeconds(seconds?: number): string {
  if (seconds === undefined) {
    return ''
  }
  if (seconds < 60) {
    return `${seconds}s`
  }
  const m = Math.floor(seconds / 60)
  return m < 60 ? `${m}m ${seconds % 60}s` : `${Math.floor(m / 60)}h ${m % 60}m`
}

export function formatSuccessRate(rate: number | null): string {
  return rate === null ? 'n/a' : `${Math.round(rate * 100)}%`
}

export function isPendingDecision(row: PipelineRunRow): boolean {
  return row.hold_pending || row.approval_pending
}

// attentionRows puts runs awaiting a decision first, then runs that failed
// in the last day, each group newest first.
export function attentionRows(
  pending: PipelineRunRow[],
  failed: PipelineRunRow[],
  now: number,
): PipelineRunRow[] {
  const byNewest = (a: PipelineRunRow, b: PipelineRunRow) =>
    b.created_at.localeCompare(a.created_at)
  const seen = new Set<string>()
  const waiting = [...pending].sort(byNewest).filter((r) => {
    if (seen.has(r.id)) {
      return false
    }
    seen.add(r.id)
    return true
  })
  const recentFailed = failed
    .filter((r) => now - new Date(r.created_at).getTime() < DAY_MS)
    .sort(byNewest)
  return [...waiting, ...recentFailed]
}

export function decisionLabel(row: PipelineRunRow): string {
  return row.hold_pending ? 'Held for approval' : 'Needs approval'
}
