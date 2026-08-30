// Query-key factory and fetcher for GET /api/v1/databases/{name}/metrics
// (internal/api/database_metrics.go): the database-kind counterpart to
// queries/metrics.ts. Same wire shape (MetricSeries/MetricName,
// types/metrics.ts) and same aggregation semantics, only the resource
// kind the resourceID resolves to differs.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { MetricName, MetricSeries } from '../types/metrics'
import { databaseKeys } from './databases'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { MetricRangeParams } from './metrics'

export const databaseMetricKeys = {
  all: (databaseName: string) =>
    [...databaseKeys.detail(databaseName), 'metrics'] as const,
  series: (
    databaseName: string,
    metric: MetricName,
    fromIso: string,
    toIso: string,
    step: string | undefined,
  ) =>
    [
      ...databaseMetricKeys.all(databaseName),
      metric,
      fromIso,
      toIso,
      step ?? 'raw',
    ] as const,
}

export async function fetchDatabaseMetricSeries(
  databaseName: string,
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
    `/api/v1/databases/${encodeURIComponent(databaseName)}/metrics?${params.toString()}`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch database metrics failed: ${res.status}`),
    )
  }
  return (await res.json()) as MetricSeries
}

export function databaseMetricSeriesQueryOptions(
  databaseName: string,
  metric: MetricName,
  range: MetricRangeParams,
) {
  return queryOptions({
    queryKey: databaseMetricKeys.series(
      databaseName,
      metric,
      range.from.toISOString(),
      range.to.toISOString(),
      range.step,
    ),
    queryFn: () => fetchDatabaseMetricSeries(databaseName, metric, range),
  })
}

export function useDatabaseMetricSeries(
  databaseName: string,
  metric: MetricName,
  range: MetricRangeParams,
  options?: { enabled?: boolean },
) {
  return useQuery({
    ...databaseMetricSeriesQueryOptions(databaseName, metric, range),
    enabled: options?.enabled ?? true,
  })
}
