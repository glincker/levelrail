// Wire type for GET /api/v1/apps-metrics (internal/api/apps_metrics_batch.go).

export interface AppMetricsSummary {
  name: string
  cpu_percent?: number
  memory_usage_bytes?: number
  memory_limit_bytes?: number
  has_traffic: boolean
  rate_per_sec: number
  error_rate_5xx: number
  p95_ms: number
  spark: number[]
  last_deploy_at?: string
}
