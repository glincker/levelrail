// Shared by ResourceLimitsEditor and DatabaseResourceLimitsEditor: a
// small, non-blocking read of how much memory/CPU the node an app or
// database currently runs on already has in use, so an operator typing a
// limit isn't guessing blind. There is no total host memory or CPU core
// count anywhere in the API (internal/api/node_metrics.go's
// nodeSummableMetrics/nodeHostMetrics deliberately excludes it, see that
// file's own doc comments: no host-level stats collection exists yet),
// so this only ever reports current in-use totals across services placed
// on the node, never a capacity figure that isn't real.
//
// Resolves to undefined whenever nodeId is missing, the node can't be
// loaded, or neither metric query produced a value: callers render
// nothing in that case, per this hint's purely-informational contract.

import { useQuery } from '@tanstack/react-query'
import { nodeDetailQueryOptions } from '../queries/nodes'
import { useNodeMetricSeries } from '../queries/nodeMetrics'
import type { MetricPoint } from '../types/metrics'

const RANGE_MS = 5 * 60 * 1000

function latestValue(points?: MetricPoint[]): number | null {
  if (!points || points.length === 0) return null
  const last = points[points.length - 1]
  return last ? last.value : null
}

export interface NodeCapacityHintData {
  nodeName: string
  memoryUsedBytes: number | null
  cpuPercent: number | null
  serviceCount: number
}

export function useNodeCapacityHint(
  nodeId: string | undefined,
): NodeCapacityHintData | undefined {
  const now = new Date()
  const range = { from: new Date(now.getTime() - RANGE_MS), to: now }

  const node = useQuery({
    ...nodeDetailQueryOptions(nodeId ?? ''),
    enabled: !!nodeId,
    retry: false,
  })
  const memory = useNodeMetricSeries(
    nodeId ?? '',
    'memory_usage_bytes',
    range,
    { enabled: !!nodeId, retry: false },
  )
  const cpu = useNodeMetricSeries(nodeId ?? '', 'cpu_percent', range, {
    enabled: !!nodeId,
    retry: false,
  })

  if (!nodeId || !node.data) {
    return undefined
  }

  const memoryUsedBytes = memory.isSuccess
    ? latestValue(memory.data.points)
    : null
  const cpuPercent = cpu.isSuccess ? latestValue(cpu.data.points) : null
  if (memoryUsedBytes === null && cpuPercent === null) {
    return undefined
  }

  return {
    nodeName: node.data.name,
    memoryUsedBytes,
    cpuPercent,
    serviceCount: Math.max(
      memory.data?.resource_count ?? 0,
      cpu.data?.resource_count ?? 0,
    ),
  }
}
