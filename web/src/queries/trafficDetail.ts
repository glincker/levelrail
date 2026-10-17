// Query-key factory and fetcher for GET /api/v1/apps/{name}/traffic
// (internal/api/traffic_app.go): RED totals, per-replica breakdown, slow
// routes, request samples and SLO burn for one app.

import { keepPreviousData, queryOptions, useQuery } from '@tanstack/react-query'
import type { TrafficDetail, TrafficWindow } from '../types/trafficDetail'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

const LIVE_REFETCH_MS = 10_000

export const trafficDetailKeys = {
  detail: (appName: string, window: TrafficWindow) =>
    [...appKeys.detail(appName), 'traffic-detail', window] as const,
}

export async function fetchTrafficDetail(
  appName: string,
  window: TrafficWindow,
): Promise<TrafficDetail> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/traffic?window=${window}`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch traffic failed: ${res.status}`),
    )
  }
  return (await res.json()) as TrafficDetail
}

export function trafficDetailQueryOptions(
  appName: string,
  window: TrafficWindow,
) {
  return queryOptions({
    queryKey: trafficDetailKeys.detail(appName, window),
    queryFn: () => fetchTrafficDetail(appName, window),
    refetchInterval: LIVE_REFETCH_MS,
    placeholderData: keepPreviousData,
  })
}

export function useTrafficDetail(appName: string, window: TrafficWindow) {
  return useQuery(trafficDetailQueryOptions(appName, window))
}
