// Wire types for GET /api/v1/nodes/{id}/metrics
// (internal/api/node_metrics.go's nodeMetricsResponse). Points reuse
// MetricPoint's shape (types/metrics.ts) directly rather than redefining
// it: the per-tick point shape (timestamp/value/count) is identical, the
// backend's metricPoint type is literally shared between both handlers.

import type { MetricPoint } from './metrics'

// Exactly internal/api/node_metrics.go's nodeSummableMetrics: the
// per-app MetricName (types/metrics.ts) minus memory_limit_bytes, which
// the backend rejects outright for a node query rather than silently
// serving, because summing per-container memory limits across a node
// does not approximate host capacity, see that map's own doc comment.
export type NodeMetricName =
  | 'cpu_percent'
  | 'memory_usage_bytes'
  | 'network_rx_bytes'
  | 'network_tx_bytes'
  | 'disk_read_bytes'
  | 'disk_write_bytes'

export interface NodeMetricSeries {
  metric: string
  points: MetricPoint[]
  // How many of the node's placed services actually contributed at
  // least one sample to this series, not how many services are placed
  // on the node in total (a placed service with no samples in range
  // contributes to neither number). Lets NodeMetricsDashboard say
  // "summed across N containers" honestly instead of implying a
  // host-level reading it can't back up.
  resource_count: number
}
