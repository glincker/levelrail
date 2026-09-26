import { useAppLastDeploy, useAppTraffic } from '../../queries/fleetTraffic'

export interface AppRowMetrics {
  loading: boolean
  hasTraffic: boolean
  spark: number[]
  p95Ms: number
  errorPct: number
  lastDeployAt: string | undefined
}

export function useAppRowMetrics(name: string): AppRowMetrics {
  const traffic = useAppTraffic(name)
  const deploys = useAppLastDeploy(name)
  const summary = traffic.data?.summary
  const latest = deploys.data?.[0]
  return {
    loading: traffic.isPending,
    hasTraffic: summary?.has_traffic ?? false,
    spark: (traffic.data?.points ?? []).map((p) => p.rate_per_sec),
    p95Ms: summary?.p95_ms ?? 0,
    errorPct: (summary?.error_rate_5xx ?? 0) * 100,
    lastDeployAt: latest
      ? (latest.finished_at ?? latest.started_at)
      : undefined,
  }
}
