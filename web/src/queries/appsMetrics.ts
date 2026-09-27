import { queryOptions, useQuery } from '@tanstack/react-query'
import type { AppMetricsSummary } from '../types/appsMetrics'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const appsMetricsKeys = {
  all: ['apps-metrics'] as const,
}

export async function fetchAppsMetrics(): Promise<AppMetricsSummary[]> {
  const res = await fetch('/api/v1/apps-metrics')
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch apps metrics failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppMetricsSummary[]
}

export function appsMetricsQueryOptions() {
  return queryOptions({
    queryKey: appsMetricsKeys.all,
    queryFn: fetchAppsMetrics,
    staleTime: 30_000,
    refetchInterval: 60_000,
    retry: false,
  })
}

export function useAppsMetrics(enabled = true) {
  return useQuery({ ...appsMetricsQueryOptions(), enabled })
}

export function useAppMetricsRow(name: string, enabled = true) {
  return useQuery({
    ...appsMetricsQueryOptions(),
    enabled,
    select: (rows) => rows.find((r) => r.name === name),
  })
}
