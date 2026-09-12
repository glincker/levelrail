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

export const appResourceUsageKeys = {
  all: ['apps', 'resource-usage'] as const,
}

export async function fetchAppResourceUsage(): Promise<AppResourceUsage[]> {
  const res = await fetch('/api/v1/apps/resource-usage')
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app resource usage failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppResourceUsage[]
}

export function appResourceUsageQueryOptions() {
  return queryOptions({
    queryKey: appResourceUsageKeys.all,
    queryFn: fetchAppResourceUsage,
  })
}

// retry: false mirrors useAppListOptional's own reasoning: a telemetry-
// not-configured 501 will never succeed on retry, so retrying just
// delays the panel quietly not rendering.
export function useAppResourceUsage() {
  return useQuery({ ...appResourceUsageQueryOptions(), retry: false })
}
