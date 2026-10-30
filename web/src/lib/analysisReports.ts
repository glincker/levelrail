import type {
  AnalysisCause,
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
