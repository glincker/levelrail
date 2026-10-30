import { useQuery } from '@tanstack/react-query'
import type {
  AnalysisReportDetail,
  AnalysisReportSummary,
} from '../types/analysisReport'
import { appKeys } from './apps'

export const analysisReportsKeys = {
  list: (appName: string) =>
    [...appKeys.detail(appName), 'analysis-reports'] as const,
  detail: (id: string) => ['analysis-report', id] as const,
}

// 404/501 mean analysis is off; treated as empty so the section stays hidden.
export async function fetchAnalysisReports(
  appName: string,
): Promise<AnalysisReportSummary[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/analysis/reports?limit=20`,
  )
  if (res.status === 404 || res.status === 501) return []
  if (!res.ok) throw new Error(`fetch analysis reports failed: ${res.status}`)
  return (await res.json()) as AnalysisReportSummary[]
}

export async function fetchAnalysisReport(
  id: string,
): Promise<AnalysisReportDetail | null> {
  const res = await fetch(`/api/v1/analysis/reports/${encodeURIComponent(id)}`)
  if (res.status === 404 || res.status === 501) return null
  if (!res.ok) throw new Error(`fetch analysis report failed: ${res.status}`)
  return (await res.json()) as AnalysisReportDetail
}

export function useAnalysisReports(appName: string) {
  return useQuery({
    queryKey: analysisReportsKeys.list(appName),
    queryFn: () => fetchAnalysisReports(appName),
    retry: false,
  })
}

export function useAnalysisReport(id: string, enabled: boolean) {
  return useQuery({
    queryKey: analysisReportsKeys.detail(id),
    queryFn: () => fetchAnalysisReport(id),
    enabled,
    retry: false,
  })
}
