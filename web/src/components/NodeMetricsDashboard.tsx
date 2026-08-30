import { useMemo, useState } from 'react'
import { PulseIcon, InfoIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { MetricChartCard } from './MetricChartCard'
import { TimeRangeControls } from './TimeRangeControls'
import { useNodeMetricSeries } from '../queries/nodeMetrics'
import type { NodeMetricName } from '../types/nodeMetrics'
import { type ChartUnit, useMergedChartQuery } from '../lib/metricChart'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type ResolvedTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'

// Node-level metrics section for routes/nodes/$id.tsx, backed by real
// data from GET /api/v1/nodes/{id}/metrics
// (internal/api/node_metrics.go). Read that handler's doc comment
// before touching this file: most charts below are the *sum* of this
// node's already-collected per-container samples
// (internal/telemetry/collector.go) for every app service currently
// placed on it, not a read of the host machine's real free/total memory
// or CPU. The Disk space group is the one exception, a genuine host
// filesystem reading (internal/telemetry/hostdisk.go's
// HostDiskCollector), which is why it carries no "summed across N
// containers" subtitle below (isHostLevel) and the alert at the bottom
// still calls out CPU/memory/network as the remaining sum-only gap.
//
// Deliberately its own component, not MetricsDashboard reused wholesale:
// the two differ enough in what they fetch (a different endpoint and
// cache-key shape, queries/nodeMetrics.ts vs queries/metrics.ts), what
// metrics they can honestly show (no memory_limit_bytes here, see
// node_metrics.go), and what they can overlay (no deploy markers,
// deploys are an app-scoped concept) that one generic component
// covering both would need more branching than either component's own
// body now has. What genuinely is shared, chart rendering and value
// formatting, lives in components/MetricChartCard.tsx and
// lib/metricChart.ts and is used by both this file and
// MetricsDashboard.tsx, so the recharts tree itself isn't duplicated a
// second time.

interface NodeSeriesConfig {
  metric: NodeMetricName
  label: string
  color: string
}

interface NodeChartGroupConfig {
  title: string
  unit: ChartUnit
  primary: NodeSeriesConfig
  secondary?: NodeSeriesConfig
  // True for the one group that reads a real host filesystem sample
  // (HostDiskCollector) rather than a sum of per-container samples:
  // suppresses the "summed across N containers" subtitle, which would
  // be misleading for a reading that was never a sum in the first
  // place.
  isHostLevel?: boolean
}

// No memory_limit_bytes group here (unlike MetricsDashboard.tsx's own
// CHART_GROUPS): the backend rejects that metric for a node query
// outright, see node_metrics.go's nodeSummableMetrics.
const NODE_CHART_GROUPS: NodeChartGroupConfig[] = [
  {
    title: 'CPU',
    unit: 'percent',
    primary: { metric: 'cpu_percent', label: 'CPU', color: '#0ea5e9' },
  },
  {
    title: 'Memory',
    unit: 'bytes',
    primary: {
      metric: 'memory_usage_bytes',
      label: 'Usage',
      color: '#0ea5e9',
    },
  },
  {
    title: 'Network I/O',
    unit: 'bytes',
    primary: {
      metric: 'network_rx_bytes',
      label: 'Received',
      color: '#22c55e',
    },
    secondary: { metric: 'network_tx_bytes', label: 'Sent', color: '#a855f7' },
  },
  {
    title: 'Disk I/O',
    unit: 'bytes',
    primary: { metric: 'disk_read_bytes', label: 'Read', color: '#22c55e' },
    secondary: {
      metric: 'disk_write_bytes',
      label: 'Write',
      color: '#a855f7',
    },
  },
  {
    title: 'Disk space',
    unit: 'bytes',
    isHostLevel: true,
    primary: { metric: 'disk_used_bytes', label: 'Used', color: '#f97316' },
    secondary: {
      metric: 'disk_total_bytes',
      label: 'Total',
      color: '#64748b',
    },
  },
]

function subtitleFor(resourceCount: number | undefined): string | undefined {
  if (resourceCount === undefined) {
    return undefined
  }
  return resourceCount === 1
    ? 'Summed across 1 container'
    : `Summed across ${resourceCount} containers`
}

// diskPercentSubtitle turns the Disk space group's merged rows into a
// "42% used" caption: the used/total lines already show the same fact
// on the chart itself, but a caller glancing at the card title
// shouldn't have to trace two lines to the right edge to get it.
function diskPercentSubtitle(
  rows: { primary?: number; secondary?: number }[],
): string | undefined {
  for (let i = rows.length - 1; i >= 0; i -= 1) {
    const used = rows[i]?.primary
    const total = rows[i]?.secondary
    if (used !== undefined && total !== undefined && total > 0) {
      return `${((used / total) * 100).toFixed(1)}% used`
    }
  }
  return undefined
}

function NodeChartCard({
  nodeId,
  group,
  range,
}: {
  nodeId: string
  group: NodeChartGroupConfig
  range: ResolvedTimeRange
}) {
  const primaryQuery = useNodeMetricSeries(nodeId, group.primary.metric, range)
  // Rules of hooks: called unconditionally regardless of `secondary`,
  // the same pattern MetricsDashboard.tsx's own ChartCard uses, so
  // `enabled` (not a conditional hook call) is what actually skips the
  // request for single-series cards like CPU/Memory here.
  const secondaryQuery = useNodeMetricSeries(
    nodeId,
    group.secondary?.metric ?? group.primary.metric,
    range,
    { enabled: Boolean(group.secondary) },
  )

  const { rows, isLoading, error } = useMergedChartQuery(
    primaryQuery,
    secondaryQuery,
    Boolean(group.secondary),
  )

  const subtitle =
    isLoading || error
      ? undefined
      : group.isHostLevel
        ? diskPercentSubtitle(rows)
        : subtitleFor(primaryQuery.data?.resource_count)

  return (
    <MetricChartCard
      title={group.title}
      unit={group.unit}
      primaryLabel={group.primary.label}
      primaryColor={group.primary.color}
      secondaryLabel={group.secondary?.label}
      secondaryColor={group.secondary?.color}
      rows={rows}
      range={range}
      isLoading={isLoading}
      error={error}
      subtitle={subtitle}
    />
  )
}

export function NodeMetricsDashboard({ nodeId }: { nodeId: string }) {
  const [rangeKey, setRangeKey] = useState<TimeRangeKey>(DEFAULT_TIME_RANGE_KEY)
  // Bumped by the Refresh button to recompute "now" without changing
  // rangeKey, the exact same refetch-loop-avoidance reasoning
  // MetricsDashboard.tsx's own header comment gives.
  const [refreshNonce, setRefreshNonce] = useState(0)
  const range = useMemo(
    () => resolveTimeRange(rangeKey),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rangeKey, refreshNonce],
  )

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <PulseIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Node metrics
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            As of {range.to.toLocaleTimeString()}. CPU, memory, and network/disk
            I/O are summed across every app container placed here; disk space is
            a real host filesystem reading.
          </p>
        </div>
        <TimeRangeControls
          rangeKey={rangeKey}
          onRangeChange={setRangeKey}
          onRefresh={() => {
            setRefreshNonce((n) => n + 1)
          }}
        />
      </div>

      <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        {NODE_CHART_GROUPS.map((group) => (
          <NodeChartCard
            key={group.title}
            nodeId={nodeId}
            group={group}
            range={range}
          />
        ))}
      </div>

      <Alert className="mt-4">
        <InfoIcon />
        <AlertTitle>What this is, and isn't</AlertTitle>
        <AlertDescription>
          <p>
            The CPU, Memory, Network I/O, and Disk I/O charts sum every app
            container's own collected stats for everything placed on this node.
            They are not a read of the host machine's actual free or total
            memory and CPU, so there is no real capacity ceiling to compare
            against there. Disk space is the exception: a real reading of the
            host filesystem the data directory lives on, not a per-container
            sum.
          </p>
          <p className="mt-2">
            Databases placed on this node aren&apos;t included in this sum: they
            have their own per-database Metrics/Logs views now, but this
            node-level total still only adds up app service containers.
          </p>
        </AlertDescription>
      </Alert>
    </section>
  )
}
