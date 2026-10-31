import { formatBytes } from './format'
import type {
  AnalysisCause,
  AnalysisLogGroup,
  AnalysisMetricStat,
  AnalysisSizing,
  AnalysisReportSummary,
} from '../types/analysisReport'

const CONFIDENCE_ORDER: Record<string, number> = { high: 3, medium: 2, low: 1 }

// Highest confidence first, then score, then the server's rank.
export function sortCauses(causes: AnalysisCause[]): AnalysisCause[] {
  return [...causes].sort(
    (a, b) =>
      (CONFIDENCE_ORDER[b.confidence] ?? 0) -
        (CONFIDENCE_ORDER[a.confidence] ?? 0) ||
      b.score - a.score ||
      a.rank - b.rank,
  )
}

export function sortReportsNewest(
  reports: AnalysisReportSummary[],
): AnalysisReportSummary[] {
  return [...reports].sort(
    (a, b) => Date.parse(b.created_at) - Date.parse(a.created_at),
  )
}

export interface LogLevelCount {
  level: string
  lines: number
  groups: number
}

// Lines and distinct groups per level, noisiest level first.
export function summarizeLogGroups(
  groups: AnalysisLogGroup[],
): LogLevelCount[] {
  const byLevel = new Map<string, LogLevelCount>()
  for (const g of groups) {
    const cur = byLevel.get(g.level) ?? { level: g.level, lines: 0, groups: 0 }
    cur.lines += g.count
    cur.groups += 1
    byLevel.set(g.level, cur)
  }
  return [...byLevel.values()].sort((a, b) => b.lines - a.lines)
}

// Metrics with no samples in the window carry no evidence.
export function usableMetrics(
  metrics: AnalysisMetricStat[] | null | undefined,
): AnalysisMetricStat[] {
  return (metrics ?? []).filter((m) => m.samples > 0)
}

export function formatSizingLimit(dimension: string, value: number): string {
  if (value <= 0) return ''
  if (dimension === 'cpu') {
    return `${Math.round((value / 1e9) * 100) / 100} cores`
  }
  return formatBytes(value)
}

// Only dimensions with a declared limit can be compared to a suggestion.
export function usableSizing(
  sizing: AnalysisSizing[] | null | undefined,
): AnalysisSizing[] {
  return (sizing ?? []).filter((s) => s.current_limit > 0)
}
