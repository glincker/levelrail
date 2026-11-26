import { keepPreviousData, useQueries } from '@tanstack/react-query'
import { databaseMetricSeriesQueryOptions } from '../../../queries/databaseMetrics'
import { pollMs } from '../../../lib/pollTier'
import type { MetricName } from '../../../types/metrics'
import type { ResolvedTimeRange } from '../../../lib/timeRange'
import type { SeriesMap } from './chartRegistry'

// One query per metric, shared by the charts, stat cards and export so the
// network sees each series once. Keeps the old data while a new window loads.
export function useMonitoringSeries(
  databaseName: string,
  metrics: readonly MetricName[],
  range: ResolvedTimeRange,
  enabled: boolean,
) {
  const results = useQueries({
    queries: metrics.map((metric) => ({
      ...databaseMetricSeriesQueryOptions(databaseName, metric, range),
      enabled,
      placeholderData: keepPreviousData,
      staleTime: pollMs('active'),
    })),
  })
  const data: SeriesMap = {}
  metrics.forEach((metric, i) => {
    const series = results[i]?.data
    if (series) data[metric] = series
  })
  return {
    data,
    isLoading: enabled && results.some((r) => r.isLoading),
    error: results.find((r) => r.error)?.error ?? null,
  }
}
