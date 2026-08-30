// Wire types for GET /api/v1/nodes/{id}/metrics
// (internal/api/node_metrics.go's nodeMetricsResponse). Points reuse
// MetricPoint's shape (types/metrics.ts) directly rather than redefining
// it: the per-tick point shape (timestamp/value/count) is identical, the
// backend's metricPoint type is literally shared between both handlers.

import type { MetricPoint } from './metrics'

// The first six mirror internal/api/node_metrics.go's
// nodeSummableMetrics exactly: the per-app MetricName (types/metrics.ts)
// minus memory_limit_bytes, which the backend rejects outright for a
// node query rather than silently serving, because summing
// per-container memory limits across a node does not approximate host
// capacity, see that map's own doc comment.
//
// disk_used_bytes/disk_total_bytes are different in kind, not summed
// across placed services at all: they're nodeHostMetrics, a direct read
// of HostDiskCollector's own real filesystem sample for this node
// (internal/telemetry/hostdisk.go), see node_metrics.go's
// writeNodeHostMetric.
export type NodeMetricName =
  | 'cpu_percent'
  | 'memory_usage_bytes'
  | 'network_rx_bytes'
  | 'network_tx_bytes'
  | 'disk_read_bytes'
  | 'disk_write_bytes'
  | 'disk_used_bytes'
  | 'disk_total_bytes'

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
