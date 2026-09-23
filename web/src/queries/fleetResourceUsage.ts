// Query-key factory and fetcher for GET /api/v1/nodes/resource-usage
// (internal/api/node_resource_usage.go), the node-scoped counterpart to
// queries/appResourceUsage.ts. Plain useQuery, not suspense: a control
// plane with no telemetry configured (501) is a real, common state, and
// both call sites (the node list columns, the dashboard fleet summary)
// should quietly not render extra data rather than block the page.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { FleetResourceUsage } from '../types/fleetResourceUsage'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const fleetResourceUsageKeys = {
  all: ['nodes', 'resource-usage'] as const,
}

export async function fetchFleetResourceUsage(): Promise<FleetResourceUsage> {
  const res = await fetch('/api/v1/nodes/resource-usage')
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch fleet resource usage failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as FleetResourceUsage
}

export function fleetResourceUsageQueryOptions() {
  return queryOptions({
    queryKey: fleetResourceUsageKeys.all,
    queryFn: fetchFleetResourceUsage,
  })
}

// 30s poll matches FleetResourceChart's own POLL_INTERVAL_MS: both read
// aggregated telemetry rather than a per-resource live stream, so a
// lightweight periodic refetch is enough, no SSE needed here. retry:
// false mirrors useAppResourceUsage's own reasoning: a telemetry-not-
// configured 501 will never succeed on retry.
const POLL_INTERVAL_MS = 30_000

export function useFleetResourceUsage() {
  return useQuery({
    ...fleetResourceUsageQueryOptions(),
    retry: false,
    refetchInterval: POLL_INTERVAL_MS,
  })
}
