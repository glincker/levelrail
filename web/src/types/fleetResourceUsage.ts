// Wire types for GET /api/v1/nodes/resource-usage
// (internal/api/node_resource_usage.go's handleFleetResourceUsage): the
// latest known CPU/memory/disk reading for every node, plus a fleet-wide
// rollup. cpu_percent/memory_usage_bytes are the sum of every service
// placed on that node's latest sample, not a true host read; the same
// "sum of containers" caveat handleQueryNodeMetrics documents for the
// per-node time-series version of this same number.
export interface NodeResourceUsage {
  node_id: string
  name: string
  is_local: boolean
  cpu_percent?: number
  memory_usage_bytes?: number
  // memory_total_bytes/disk_used_bytes/disk_total_bytes are real host
  // reads, but (today) only ever populated for the node running the
  // control plane itself: undefined here means "no collector for this
  // node," not "empty disk."
  memory_total_bytes?: number
  disk_used_bytes?: number
  disk_total_bytes?: number
}

// FleetResourceUsageRollup's *_percent fields are only computed from
// nodes that actually reported a capacity figure: nodes_with_*_capacity
// says how many of node_count that covers, and UI code must show that
// coverage alongside the percentage rather than implying it reflects the
// whole fleet.
export interface FleetResourceUsageRollup {
  node_count: number
  total_cpu_percent?: number
  total_memory_usage_bytes?: number
  total_memory_bytes?: number
  memory_used_percent?: number
  nodes_with_memory_capacity: number
  total_disk_used_bytes?: number
  total_disk_bytes?: number
  disk_used_percent?: number
  nodes_with_disk_capacity: number
}

export interface FleetResourceUsage {
  nodes: NodeResourceUsage[]
  fleet: FleetResourceUsageRollup
}
