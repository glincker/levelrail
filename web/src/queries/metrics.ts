// Query-key factory and fetcher for GET /api/v1/apps/{name}/metrics
// (internal/api/metrics.go, TASKS.md 2.3). Kept in its own module
// rather than folded into queries/apps.ts, same reasoning queries/
// deploys.ts's header comment already gives: a genuinely different
// resource shape (a metric series, not an AppDetail), even though it
// nests under the same app name.
//
// Not built on useSuspenseQuery like appDetailQueryOptions/
// deployStatusQueryOptions: those two are primed in the route loader
// before first paint, but MetricsDashboard's queries depend on
// component-local state (the selected time range) that doesn't exist
// until the component has mounted, so a plain useQuery with its own
// loading/error rendering is the right fit here, not a loader-primed
// suspense query.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { MetricName, MetricSeries } from '../types/metrics'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const metricKeys = {
  all: (appName: string) => [...appKeys.detail(appName), 'metrics'] as const,
  series: (
    appName: string,
    metric: MetricName,
    fromIso: string,
    toIso: string,
    step: string | undefined,
  ) =>
    [
      ...metricKeys.all(appName),
      metric,
      fromIso,
      toIso,
      step ?? 'raw',
    ] as const,
}

export interface MetricRangeParams {
  from: Date
  to: Date
  /** A Go duration string (e.g. "60s"); omitted means raw samples. */
  step?: string
}

export async function fetchMetricSeries(
  appName: string,
  metric: MetricName,
  range: MetricRangeParams,
): Promise<MetricSeries> {
  const params = new URLSearchParams({
    metric,
    from: range.from.toISOString(),
    to: range.to.toISOString(),
  })
  if (range.step) {
    params.set('step', range.step)
  }
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/metrics?${params.toString()}`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch metrics failed: ${res.status}`),
    )
  }
  return (await res.json()) as MetricSeries
}

export function metricSeriesQueryOptions(
  appName: string,
  metric: MetricName,
  range: MetricRangeParams,
) {
  return queryOptions({
    queryKey: metricKeys.series(
      appName,
      metric,
      range.from.toISOString(),
      range.to.toISOString(),
      range.step,
    ),
    queryFn: () => fetchMetricSeries(appName, metric, range),
  })
}

export function useMetricSeries(
  appName: string,
  metric: MetricName,
  range: MetricRangeParams,
  options?: { enabled?: boolean },
) {
  return useQuery({
    ...metricSeriesQueryOptions(appName, metric, range),
    enabled: options?.enabled ?? true,
  })
}
