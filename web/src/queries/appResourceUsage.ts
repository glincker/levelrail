// Query-key factory and fetcher for GET /api/v1/apps/resource-usage
// (internal/api/app_resource_usage.go), the dashboard-wide "what's
// consuming the most CPU/memory/traffic right now" ranking. Plain
// useQuery, not suspense: a control plane with no telemetry configured
// (501) is a real, common state (see fetchAppResourceUsage's own 501
// handling), and the panel reading this should quietly not render
// rather than block the whole dashboard.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { AppResourceUsage } from '../types/appResourceUsage'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { DEDUPE_STALE_MS } from '../lib/queryStale'
import { useEnvironmentScope } from '../lib/environmentScope'
import {
  currentScope,
  scopeKey,
  scopeUrl,
  type ScopeParams,
} from '../lib/scopeQuery'

export const appResourceUsageKeys = {
  all: ['apps', 'resource-usage'] as const,
}

export async function fetchAppResourceUsage(
  scope: ScopeParams = currentScope(),
): Promise<AppResourceUsage[]> {
  const res = await fetch(scopeUrl('/api/v1/apps/resource-usage', scope))
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch app resource usage failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AppResourceUsage[]
}

export function appResourceUsageQueryOptions() {
  const scope = currentScope()
  return queryOptions({
    queryKey: [...appResourceUsageKeys.all, ...scopeKey(scope)] as const,
    queryFn: () => fetchAppResourceUsage(scope),
    staleTime: DEDUPE_STALE_MS,
  })
}

// retry: false mirrors useAppListOptional's own reasoning: a telemetry-
// not-configured 501 will never succeed on retry, so retrying just
// delays the panel quietly not rendering.
export function useAppResourceUsage() {
  useEnvironmentScope()
  return useQuery({ ...appResourceUsageQueryOptions(), retry: false })
}
