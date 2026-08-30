import { useMemo, useState } from 'react'
import { PulseIcon } from '@phosphor-icons/react/dist/ssr'
import { MetricChartCard } from './MetricChartCard'
import { TimeRangeControls } from './TimeRangeControls'
import { useDatabaseMetricSeries } from '../queries/databaseMetrics'
import type { MetricName } from '../types/metrics'
import { type ChartUnit, useMergedChartQuery } from '../lib/metricChart'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type ResolvedTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'

// Per-database metrics dashboard: GET /api/v1/databases/{name}/metrics
// (internal/api/database_metrics.go), the database-kind counterpart to
// MetricsDashboard.tsx (per-app). Deliberately its own component rather
// than that one reused wholesale, the same reasoning
// NodeMetricsDashboard.tsx's own header comment gives for its own case:
// a database has no deploy-attempt history to overlay as markers
// (deploys are an app-scoped concept, internal/api/deploy_attempts.go),
// and none of MetricsDashboard's "not yet collected" list (request
// rate, response time percentiles, error rate) is a meaningful gap to
// call out for a database, which never terminates HTTP requests itself.
// The chart rendering itself (MetricChartCard, lib/metricChart.ts) is
// still fully shared, only the data-fetching and page chrome differ.

interface SeriesConfig {
  metric: MetricName
  label: string
  color: string
}

interface ChartGroupConfig {
  title: string
  unit: ChartUnit
  primary: SeriesConfig
  secondary?: SeriesConfig
}

const CHART_GROUPS: ChartGroupConfig[] = [
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
    secondary: {
      metric: 'memory_limit_bytes',
      label: 'Limit',
      color: '#f59e0b',
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

function ChartCard({
  databaseName,
  group,
  range,
}: {
  databaseName: string
  group: ChartGroupConfig
  range: ResolvedTimeRange
}) {
  const primaryQuery = useDatabaseMetricSeries(
    databaseName,
    group.primary.metric,
    range,
  )
  // Rules of hooks: called unconditionally regardless of `secondary`,
  // the same pattern MetricsDashboard.tsx's own ChartCard uses.
  const secondaryQuery = useDatabaseMetricSeries(
    databaseName,
    group.secondary?.metric ?? group.primary.metric,
    range,
    { enabled: Boolean(group.secondary) },
  )

  const { rows, isLoading, error } = useMergedChartQuery(
    primaryQuery,
    secondaryQuery,
    Boolean(group.secondary),
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
    />
  )
}

export function DatabaseMetricsDashboard({
  databaseName,
}: {
  databaseName: string
}) {
  const [rangeKey, setRangeKey] = useState<TimeRangeKey>(DEFAULT_TIME_RANGE_KEY)
  // Bumped by the Refresh button to recompute "now" without changing
  // rangeKey, the same refetch-loop-avoidance reasoning
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
            Metrics
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            As of {range.to.toLocaleTimeString()}.
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
        {CHART_GROUPS.map((group) => (
          <ChartCard
            key={group.title}
            databaseName={databaseName}
            group={group}
            range={range}
          />
        ))}
      </div>
    </section>
  )
}
