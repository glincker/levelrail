// Shapes from the build-report and next-advisor endpoints, hand-written because
// the generated openapi types do not cover them yet.
import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type RouteKind = 'static' | 'isr' | 'dynamic' | 'unknown'

export interface BuildReportRoute {
  path: string
  kind: RouteKind
  size_kb: number
  first_load_kb: number
}

export interface BuildReport {
  detected: boolean
  routes: BuildReportRoute[]
  shared_first_load_kb: number | null
  warnings: string[]
}

export type AdvisorSeverity = 'info' | 'warn' | 'error'

export interface AdvisorFinding {
  id: string
  severity: AdvisorSeverity
  title: string
  detail: string
  fix: string
}

export interface NextAdvisor {
  detected: boolean
  findings: AdvisorFinding[]
}

async function getJson<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export const nextReportKeys = {
  report: (app: string, deployId: string) =>
    ['apps', app, 'deploys', deployId, 'build-report'] as const,
  advisor: (app: string) => ['apps', app, 'next-advisor'] as const,
}

export function useBuildReport(app: string, deployId: string) {
  return useQuery({
    queryKey: nextReportKeys.report(app, deployId),
    queryFn: () =>
      getJson<BuildReport>(
        `/api/v1/apps/${encodeURIComponent(app)}/deploys/${encodeURIComponent(deployId)}/build-report`,
      ),
    retry: false,
  })
}

export function useNextAdvisor(app: string) {
  return useQuery({
    queryKey: nextReportKeys.advisor(app),
    queryFn: () =>
      getJson<NextAdvisor>(
        `/api/v1/apps/${encodeURIComponent(app)}/next-advisor`,
      ),
    retry: false,
  })
}
