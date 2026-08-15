// Query-key factory and fetcher for GET /api/v1/nodes/{id}/metrics
// (internal/api/node_metrics.go), mirroring queries/metrics.ts's own
// shape exactly (queryOptions + plain useQuery, not suspense, since this
// depends on component-local time-range state that doesn't exist before
// mount, the same reasoning that file's own header comment gives).

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { NodeMetricName, NodeMetricSeries } from '../types/nodeMetrics'
import type { MetricRangeParams } from './metrics'
import { nodeKeys } from './nodes'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const nodeMetricKeys = {
  all: (nodeId: string) => [...nodeKeys.detail(nodeId), 'metrics'] as const,
  series: (
    nodeId: string,
    metric: NodeMetricName,
    fromIso: string,
    toIso: string,
    step: string | undefined,
  ) =>
    [
      ...nodeMetricKeys.all(nodeId),
      metric,
      fromIso,
      toIso,
      step ?? 'raw',
    ] as const,
}

export async function fetchNodeMetricSeries(
  nodeId: string,
  metric: NodeMetricName,
  range: MetricRangeParams,
): Promise<NodeMetricSeries> {
  const params = new URLSearchParams({
    metric,
    from: range.from.toISOString(),
    to: range.to.toISOString(),
  })
  if (range.step) {
    params.set('step', range.step)
  }
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(nodeId)}/metrics?${params.toString()}`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch node metrics failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeMetricSeries
}

export function nodeMetricSeriesQueryOptions(
  nodeId: string,
  metric: NodeMetricName,
  range: MetricRangeParams,
) {
  return queryOptions({
    queryKey: nodeMetricKeys.series(
      nodeId,
      metric,
      range.from.toISOString(),
      range.to.toISOString(),
      range.step,
    ),
    queryFn: () => fetchNodeMetricSeries(nodeId, metric, range),
  })
}

export function useNodeMetricSeries(
  nodeId: string,
  metric: NodeMetricName,
  range: MetricRangeParams,
  options?: { enabled?: boolean },
) {
  return useQuery({
    ...nodeMetricSeriesQueryOptions(nodeId, metric, range),
    enabled: options?.enabled ?? true,
  })
}
