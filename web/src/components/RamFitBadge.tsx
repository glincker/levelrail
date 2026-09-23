import { useMemo } from 'react'
import { Badge } from '@/components/ui/badge'
import { latestValue } from '../hooks/useNodeCapacityHint'
import { formatBytes } from '../lib/format'
import { useNodeListOptional } from '../queries/nodes'
import { useNodeMetricSeries } from '../queries/nodeMetrics'

// Live counterpart to a service template's static RecommendedMemoryBytes
// advisory (internal/catalog.Template): compares it against the local
// node's real, currently-available memory (HostMemoryCollector,
// internal/telemetry/hostmemory.go). Only the local node has real host
// memory data (see that collector's own doc comment on scope), so this
// renders nothing for any other deployment shape.
export function RamFitBadge({
  recommendedMemoryBytes,
}: {
  recommendedMemoryBytes: number
}) {
  const nodesQuery = useNodeListOptional()
  const localNode = nodesQuery.data?.find((n) => n.is_local)

  // Fixed once per mount, not recomputed on every render: a fresh Date
  // here would change useNodeMetricSeries's query key each render (its
  // key includes from/to as ISO strings), so the query never settles
  // and this badge never stops flickering between loading and shown.
  const range = useMemo(() => {
    const to = new Date()
    return { from: new Date(to.getTime() - 5 * 60 * 1000), to }
  }, [])
  const memoryQuery = useNodeMetricSeries(
    localNode?.id ?? '',
    'memory_available_bytes',
    range,
    { enabled: !!localNode, retry: false },
  )

  if (!localNode || memoryQuery.isLoading || memoryQuery.isError) {
    return null
  }

  const available = latestValue(memoryQuery.data?.points)
  if (available === null) {
    return null
  }

  const fits = available >= recommendedMemoryBytes

  return (
    <Badge variant={fits ? 'success' : 'warning'} className="text-xs">
      {fits
        ? `Fits on ${localNode.name} (${formatBytes(available)} free)`
        : `May not fit on ${localNode.name}: only ${formatBytes(available)} free`}
    </Badge>
  )
}
