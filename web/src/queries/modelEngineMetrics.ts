// Query for /api/v1/models/{name}/engine-metrics (internal/api/models_metrics.go).

import { useQuery } from '@tanstack/react-query'
import type { EngineMetricsReport } from '../types/models'
import { modelKeys, requestJson } from './models'

const ENGINE_METRICS_REFETCH_MS = 15000

export const modelEngineMetricsKey = (name: string, hours: number) =>
  [...modelKeys.all, 'engine-metrics', name, hours] as const

export function useModelEngineMetrics(name: string, hours: number) {
  return useQuery({
    queryKey: modelEngineMetricsKey(name, hours),
    queryFn: () =>
      requestJson<EngineMetricsReport>(
        `/api/v1/models/${encodeURIComponent(name)}/engine-metrics?since=${String(hours)}h`,
        undefined,
        'fetch engine metrics',
      ),
    refetchInterval: ENGINE_METRICS_REFETCH_MS,
  })
}
