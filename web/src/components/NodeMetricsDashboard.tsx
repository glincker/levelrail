import { useMemo, useState } from 'react'
import {
  PulseIcon,
  InfoIcon,
  ArrowsClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from './ui/button'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { MetricChartCard } from './MetricChartCard'
import { useNodeMetricSeries } from '../queries/nodeMetrics'
import type { NodeMetricName } from '../types/nodeMetrics'
import { type ChartUnit, mergeMetricSeries } from '../lib/metricChart'
import {
  DEFAULT_TIME_RANGE_KEY,
  TIME_RANGE_PRESETS,
  resolveTimeRange,
  type ResolvedTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'

// Node-level metrics section for routes/nodes/$id.tsx, backed by real
// data from GET /api/v1/nodes/{id}/metrics
// (internal/api/node_metrics.go). Read that handler's doc comment
// before touching this file, because "node-level" here is a specific,
// narrower claim than it sounds like: every chart below is the *sum* of
// this node's already-collected per-container samples
// (internal/telemetry/collector.go) for every app service currently
// placed on it, not a read of the host machine's real free/total memory
// or CPU. internal/agent has no host-level stats collection today (no
// /proc reads, no host-info gRPC message in internal/agent/agentpb),
// and internal/docker.ContainerStats is entirely per-container by
// design, so a true "how full is this box" view is real, separate
// agent-side work this pass didn't do. The alert at the bottom of this
// component says so explicitly, the same NOT_YET_COLLECTED honesty
// MetricsDashboard.tsx (the per-app dashboard this mirrors) already
// established for its own gaps, rather than a node total quietly
// implying a host reading it can't back up.
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
]

function subtitleFor(resourceCount: number | undefined): string | undefined {
  if (resourceCount === undefined) {
    return undefined
  }
  return resourceCount === 1
    ? 'Summed across 1 container'
    : `Summed across ${resourceCount} containers`
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

  const isLoading =
    primaryQuery.isLoading ||
    (Boolean(group.secondary) && secondaryQuery.isLoading)
  const error =
    primaryQuery.error ?? (group.secondary ? secondaryQuery.error : null)

  const rows = useMemo(
    () =>
      mergeMetricSeries(
        primaryQuery.data,
        group.secondary ? secondaryQuery.data : undefined,
      ),
    [primaryQuery.data, secondaryQuery.data, group.secondary],
  )

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
      subtitle={
        isLoading || error
          ? undefined
          : subtitleFor(primaryQuery.data?.resource_count)
      }
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
            As of {range.to.toLocaleTimeString()}. Sum of every app
            container placed on this node, not a host-level reading.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div
            role="group"
            aria-label="Time range"
            className="inline-flex rounded-md border border-border"
          >
            {TIME_RANGE_PRESETS.map((preset) => (
              <button
                key={preset.key}
                type="button"
                onClick={() => {
                  setRangeKey(preset.key)
                }}
                aria-pressed={rangeKey === preset.key}
                className={`px-2.5 py-1 text-xs font-medium first:rounded-l-md last:rounded-r-md ${
                  rangeKey === preset.key
                    ? 'bg-foreground text-background'
                    : 'bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground'
                }`}
              >
                {preset.label}
              </button>
            ))}
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              setRefreshNonce((n) => n + 1)
            }}
          >
            <ArrowsClockwiseIcon className="size-3.5" aria-hidden="true" />
            Refresh
          </Button>
        </div>
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
            These charts sum every app container's own collected CPU,
            memory, and network/disk I/O for everything placed on this
            node. They are not a read of the host machine's actual free
            or total memory and CPU: this platform has no host-level
            stats agent yet (Docker's per-container stats API doesn't
            expose that), so there is no real capacity ceiling to
            compare against here.
          </p>
          <p className="mt-2">
            Databases placed on this node aren&apos;t included: only app
            service containers are collected today, the same gap the
            per-app metrics view documents.
          </p>
        </AlertDescription>
      </Alert>
    </section>
  )
}
