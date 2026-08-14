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

// The 7 metrics internal/telemetry's collector (TASKS.md 2.1) actually
// writes samples for, matching internal/api/metrics.go's `metric` query
// param one-to-one.
//
// Request rate, response time percentiles, error rate, container
// restart count, build duration, and deploy frequency are also required
// per-app metrics that must exist without configuration. None of those
// six exist here: TASKS.md 2.1 explicitly deferred restart count/build
// duration/deploy frequency as their own follow-up, and request
// rate/response time/error rate need ingress-layer instrumentation that
// doesn't exist yet either (the embedded Caddy driver has no
// request-metrics hook wired up). Do not add a MetricName for any of
// those until a real collector backs it. MetricsDashboard.tsx shows a
// clearly labeled "not yet collected" list for this gap instead of
// rendering an empty or fabricated chart.
export type MetricName =
  | 'cpu_percent'
  | 'memory_usage_bytes'
  | 'memory_limit_bytes'
  | 'network_rx_bytes'
  | 'network_tx_bytes'
  | 'disk_read_bytes'
  | 'disk_write_bytes'
