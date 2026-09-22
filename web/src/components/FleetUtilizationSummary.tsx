import { GaugeIcon } from '@phosphor-icons/react/dist/ssr'
import { useFleetResourceUsage } from '../queries/fleetResourceUsage'
import { formatBytes } from '../lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'

// Compact fleet-wide "how full are my servers" summary for the
// dashboard, sitting alongside FleetResourceChart's per-app trend: that
// component answers "which app is using resources," this one answers
// "how much headroom does the fleet itself have left." Renders nothing
// while pending, on error, or with no nodes, the same quiet-no-render
// shape FleetResourceChart already uses for a telemetry-not-configured
// (501) control plane.
export function FleetUtilizationSummary() {
  const { data, isPending, isError } = useFleetResourceUsage()

  if (isPending || isError || !data || data.fleet.node_count === 0) {
    return null
  }

  const { fleet } = data

  return (
    <Card size="sm">
      <CardHeader className="flex-row items-center justify-between gap-3 space-y-0">
        <CardTitle className="flex items-center gap-1.5 text-sm font-medium">
          <GaugeIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          Fleet utilization
        </CardTitle>
        <span className="text-xs text-muted-foreground">
          {fleet.node_count} node{fleet.node_count === 1 ? '' : 's'}
        </span>
      </CardHeader>
      <CardContent className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <CPUSummary
          totalCpuPercent={fleet.total_cpu_percent}
          nodeCount={fleet.node_count}
        />
        <CapacitySummary
          label="Memory"
          usedBytes={fleet.total_memory_usage_bytes}
          totalBytes={fleet.total_memory_bytes}
          usedPercent={fleet.memory_used_percent}
          nodesWithCapacity={fleet.nodes_with_memory_capacity}
          nodeCount={fleet.node_count}
        />
        <CapacitySummary
          label="Disk"
          usedBytes={fleet.total_disk_used_bytes}
          totalBytes={fleet.total_disk_bytes}
          usedPercent={fleet.disk_used_percent}
          nodesWithCapacity={fleet.nodes_with_disk_capacity}
          nodeCount={fleet.node_count}
        />
      </CardContent>
    </Card>
  )
}

// CPU has no known fleet capacity (no node reports core count, see
// FleetResourceUsageRollup's own doc comment), so this shows the raw
// summed percentage as a number rather than a bar against an unknown
// denominator, distinct from the Memory/Disk cards below which do have a
// real capacity to draw a bar against.
function CPUSummary({
  totalCpuPercent,
  nodeCount,
}: {
  totalCpuPercent?: number
  nodeCount: number
}) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">CPU (sum of containers)</p>
      {totalCpuPercent === undefined ? (
        <p className="mt-1 text-sm text-muted-foreground">No data yet</p>
      ) : (
        <p className="mt-1 font-mono text-lg font-semibold text-foreground">
          {totalCpuPercent.toFixed(1)}%
          <span className="ml-1.5 text-xs font-normal text-muted-foreground">
            across {nodeCount} node{nodeCount === 1 ? '' : 's'}
          </span>
        </p>
      )}
    </div>
  )
}

// nodesWithCapacity/nodeCount is always shown next to the percentage
// (see FleetResourceUsageRollup's own doc comment): a 3-node fleet where
// only 1 node has a host-metrics collector must never read as if the bar
// covers all 3, since that would silently misrepresent real headroom.
function CapacitySummary({
  label,
  usedBytes,
  totalBytes,
  usedPercent,
  nodesWithCapacity,
  nodeCount,
}: {
  label: string
  usedBytes?: number
  totalBytes?: number
  usedPercent?: number
  nodesWithCapacity: number
  nodeCount: number
}) {
  const coverage =
    nodesWithCapacity < nodeCount
      ? ` (${nodesWithCapacity}/${nodeCount} nodes reporting)`
      : ''

  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      {usedPercent === undefined || totalBytes === undefined ? (
        <p className="mt-1 text-sm text-muted-foreground">
          No capacity data{coverage}
        </p>
      ) : (
        <>
          <p className="mt-1 font-mono text-lg font-semibold text-foreground">
            {usedPercent.toFixed(1)}%
          </p>
          <Progress value={Math.min(usedPercent, 100)} className="mt-1" />
          <p className="mt-1 text-xs text-muted-foreground">
            {formatBytes(usedBytes)} / {formatBytes(totalBytes)}
            {coverage}
          </p>
        </>
      )}
    </div>
  )
}
