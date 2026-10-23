import { queryOptions, useQuery } from '@tanstack/react-query'
import type { AppMetricsSummary } from '../types/appsMetrics'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { useEnvironmentScope } from '../lib/environmentScope'
import {
  currentScope,
  scopeKey,
  scopeUrl,
  type ScopeParams,
} from '../lib/scopeQuery'

export const appsMetricsKeys = {
  all: ['apps-metrics'] as const,
}

export async function fetchAppsMetrics(
  scope: ScopeParams = currentScope(),
): Promise<AppMetricsSummary[]> {
  const res = await fetch(scopeUrl('/api/v1/apps-metrics', scope))
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
  const scope = currentScope()
  return queryOptions({
    queryKey: [...appsMetricsKeys.all, ...scopeKey(scope)] as const,
    queryFn: () => fetchAppsMetrics(scope),
    staleTime: 30_000,
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
    retry: false,
  })
}

export function useAppsMetrics(enabled = true) {
  useEnvironmentScope()
  return useQuery({ ...appsMetricsQueryOptions(), enabled })
}

export function useAppMetricsRow(name: string, enabled = true) {
  useEnvironmentScope()
  return useQuery({
    ...appsMetricsQueryOptions(),
    enabled,
    select: (rows) => rows.find((r) => r.name === name),
  })
}
