import { useNodeCapacityHint } from '../hooks/useNodeCapacityHint'
import { formatBytes } from '../lib/format'

// Renders next to the memory or CPU limit field in ResourceLimitsEditor
// and DatabaseResourceLimitsEditor: purely informational context on what
// the node is already using, never a cap. Renders nothing when the node
// is unassigned, unresolvable, or has no usable metric for this
// dimension, so it never implies a broken or misleading reading.
export function NodeCapacityHint({
  nodeId,
  dimension,
}: {
  nodeId?: string
  dimension: 'memory' | 'cpu'
}) {
  const hint = useNodeCapacityHint(nodeId)
  if (!hint) return null

  const value =
    dimension === 'memory' ? hint.memoryUsedBytes : hint.cpuPercent
  if (value === null) return null

  const formatted =
    dimension === 'memory' ? formatBytes(value) : `${value.toFixed(1)}%`
  const label = dimension === 'memory' ? 'memory' : 'CPU'
  const services =
    hint.serviceCount > 0
      ? ` across ${hint.serviceCount} service${hint.serviceCount === 1 ? '' : 's'}`
      : ''

  return (
    <p className="mt-1 text-xs text-muted-foreground">
      Node &quot;{hint.nodeName}&quot; currently has {formatted} {label} in
      use{services}.
    </p>
  )
}
