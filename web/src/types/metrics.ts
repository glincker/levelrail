// Wire types for GET /api/v1/apps/{name}/metrics
// (internal/api/metrics.go's metricsResponse/metricPoint). A point's
// `value` is already a step-bucketed average when the request passed a
// `step`, or a raw per-sample value when it didn't (telemetry.Aggregate's
// own step<=0 contract); this type doesn't distinguish the two cases,
// the caller controls that by whether it passed `step` (see
// queries/metrics.ts).

export interface MetricPoint {
  timestamp: string
  value: number
  count: number
}

export interface MetricSeries {
  metric: string
  points: MetricPoint[]
}

// The 9 metrics internal/telemetry actually writes samples for,
// matching internal/api/metrics.go's `metric` query param one-to-one.
// 7 come from a Docker stats poll (internal/telemetry/collector.go's
// sampleValues); container_restart_count and build_duration_seconds are
// discrete events, each recorded once, at the moment it happens, by its
// own caller (internal/alerting.RestartTracker for a real container
// restart, internal/deploy for a completed build), the same shape
// deploy_count already established in internal/telemetry/
// deploy_metrics.go before either of these two existed.
//
// Request rate, response time percentiles, and error rate are also
// required per-app metrics that must exist without configuration, but
// none of those three exist here: they need ingress-layer
// instrumentation that doesn't exist yet (the embedded Caddy driver has
// no request-metrics hook wired up). Do not add a MetricName for any of
// those until a real collector backs it. MetricsDashboard.tsx shows a
// clearly labeled "not yet collected" list for this remaining gap
// instead of rendering an empty or fabricated chart.
export type MetricName =
  | 'cpu_percent'
  | 'memory_usage_bytes'
  | 'memory_limit_bytes'
  | 'network_rx_bytes'
  | 'network_tx_bytes'
  | 'disk_read_bytes'
  | 'disk_write_bytes'
  | 'container_restart_count'
  | 'build_duration_seconds'
