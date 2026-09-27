import { useAppMetricsRow } from '../../queries/appsMetrics'

export interface AppRowMetrics {
  loading: boolean
  hasTraffic: boolean
  spark: number[]
  p95Ms: number
  errorPct: number
  lastDeployAt: string | undefined
}

export function useAppRowMetrics(name: string, enabled = true): AppRowMetrics {
  const row = useAppMetricsRow(name, enabled)
  const data = row.data
  return {
    loading: enabled && row.isPending,
    hasTraffic: data?.has_traffic ?? false,
    spark: data?.spark ?? [],
    p95Ms: data?.p95_ms ?? 0,
    errorPct: (data?.error_rate_5xx ?? 0) * 100,
    lastDeployAt: data?.last_deploy_at,
  }
}
