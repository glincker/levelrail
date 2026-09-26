// Query-key factory and fetcher for GET /api/v1/apps/{name}/requests
// (internal/api/requests.go): ingress request rate, error rates and
// latency percentiles for one app.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { RequestSeries } from '../types/requests'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const requestKeys = {
  series: (
    appName: string,
    fromIso: string,
    toIso: string,
    step: string | undefined,
  ) =>
    [
      ...appKeys.detail(appName),
      'requests',
      fromIso,
      toIso,
      step ?? 'auto',
    ] as const,
}

export interface RequestRangeParams {
  from: Date
  to: Date
  step?: string
}

export async function fetchRequestSeries(
  appName: string,
  range: RequestRangeParams,
): Promise<RequestSeries> {
  const params = new URLSearchParams({
    from: range.from.toISOString(),
    to: range.to.toISOString(),
  })
  if (range.step) {
    params.set('step', range.step)
  }
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/requests?${params.toString()}`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch requests failed: ${res.status}`),
    )
  }
  return (await res.json()) as RequestSeries
}

export function requestSeriesQueryOptions(
  appName: string,
  range: RequestRangeParams,
) {
  return queryOptions({
    queryKey: requestKeys.series(
      appName,
      range.from.toISOString(),
      range.to.toISOString(),
      range.step,
    ),
    queryFn: () => fetchRequestSeries(appName, range),
  })
}

export function useRequestSeries(appName: string, range: RequestRangeParams) {
  return useQuery(requestSeriesQueryOptions(appName, range))
}
