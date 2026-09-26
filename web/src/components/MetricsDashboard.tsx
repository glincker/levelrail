import { useMemo, useState } from 'react'
import { PulseIcon } from '@phosphor-icons/react/dist/ssr'
import { RequestMetricsSection } from './RequestMetricsSection'
import { MetricChartCard } from './MetricChartCard'
import { TimeRangeControls } from './TimeRangeControls'
import { useMetricSeries } from '../queries/metrics'
import type { MetricName } from '../types/metrics'
import type { DeployAttempt, DeployAttemptStatus } from '../types/deployAttempt'
import {
  type ChartMarker,
  type ChartUnit,
  useMergedChartQuery,
} from '../lib/metricChart'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type ResolvedTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'

// Per-app metrics dashboard, wired against the real
// `GET /api/v1/apps/{name}/metrics`. Request rate, error rate and latency
// come from the ingress via RequestMetricsSection.
//
// Container restart count is real but deliberately not a CHART_GROUPS
// line chart: internal/telemetry.MetricContainerRestartCount is one
// sample per real restart (Value always 1, the same discrete-event
// shape as deploy_count), so a line chart of it would just be a flat
// row of 1s, not a trend worth plotting. restartCountLabel below reads
// the same way deployMarkers.length already does for deploy frequency:
// the number of samples in the visible range *is* the restart count for
// that range, shown as a stat next to deploy frequency, not a chart.
//
// Build duration (build_duration_seconds) is the opposite case: each
// sample's Value is a real, varying number (one build's wall-clock
// duration), so it renders as a normal CHART_GROUPS line chart below.
//
// "Deploy markers overlaid on metric charts" (Phase 2's own framing:
// the feature that makes "which deploy caused this" a visual question
// instead of an investigation) used to be a single-marker
// approximation, because GET /api/v1/apps/{name}/deploys
// (internal/api/deploys.go's handleDeployHistory) only ever returns the
// *current* reconcile condition per (controller, type) pair, with no
// attempt-by-attempt history to plot. That gap is closed: this
// component now reads internal/api/deploy_attempts.go's
// handleListDeployAttempts (GET /api/v1/apps/{name}/deploy-attempts,
// wired via queries/deployAttempts.ts), a real row-per-attempt history,
// newest first, with no pagination. resolveDeployMarkers below filters
// that list to whatever falls inside the chart's currently resolved
// visible time range and plots one ReferenceLine per attempt,
// color-coded by DEPLOY_MARKER_COLOR (green succeeded, red failed, gray
// running), with a native SVG <title> on each marker's dot so hovering
// it shows the attempt's image tag and start time.
//
// handleDeployHistory/GET /apps/{name}/deploys is deliberately
// untouched by this change and stays current-status-only: it remains
// the source for useDeployStatus, ConditionsPanel, and
// AppScopedSidebar's status badge, a separate concern from
// deploy_attempts.go's own doc comment explaining why the two endpoints
// are not merged.
//
// Deploy frequency, formerly listed below as an uncollected metric, is
// now directly computable from the same deploy-attempts list with no
// new backend work: deployFrequencyLabel below counts attempts whose
// started_at falls in the visible range, the same filter
// resolveDeployMarkers already applies for the chart overlay.

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

// Colors are fixed hex values, not Tailwind theme tokens: recharts draws
// to an SVG canvas outside Tailwind's class-based theming, so a color
// that reads fine in both light and dark mode has to be picked directly
// rather than referenced by class name.
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
  {
    title: 'Build duration',
    unit: 'seconds',
    primary: {
      metric: 'build_duration_seconds',
      label: 'Duration',
      color: '#0ea5e9',
    },
  },
]

// resolveDeployMarkers filters appName's real deploy-attempt history
// (see this file's header comment) down to whatever falls inside the
// chart's currently resolved visible time range, one marker per
// attempt, oldest first. This is the real, multi-marker replacement for
// the old single "Ready" condition approximation: every attempt that
// was started while the chart is looking gets its own marker, not just
// the most recent one. Returns MetricChartCard's own generic
// ChartMarker shape directly (lib/metricChart.ts) rather than a
// DeployAttempt-specific type, since the chart rendering itself
// (MetricChartCard.tsx) has no notion of what a "deploy" is.
function resolveDeployMarkers(
  attempts: DeployAttempt[],
  range: ResolvedTimeRange,
): ChartMarker[] {
  const markers: ChartMarker[] = []
  for (const attempt of attempts) {
    const t = Date.parse(attempt.started_at)
    if (Number.isNaN(t)) {
      continue
    }
    if (t < range.from.getTime() || t > range.to.getTime()) {
      continue
    }
    markers.push({
      key: attempt.id,
      t,
      color: DEPLOY_MARKER_COLOR[attempt.status],
      tooltip: formatDeployMarkerTooltip(attempt, t),
    })
  }
  return markers.sort((a, b) => a.t - b.t)
}

// Deploy markers are color-coded by status so a failed deploy is a
// visually distinct signal from a routine one, making "which deploy
// caused this" a visual question, matching DeployAttemptsList.tsx's own
// succeeded/failed/running badge colors so the same status reads the
// same way in both places.
const DEPLOY_MARKER_COLOR: Record<DeployAttemptStatus, string> = {
  succeeded: '#22c55e',
  failed: '#ef4444',
  running: '#94a3b8',
  held: '#f59e0b',
  superseded: '#94a3b8',
}

const DEPLOY_MARKER_STATUS_LABEL: Record<DeployAttemptStatus, string> = {
  succeeded: 'Succeeded',
  failed: 'Failed',
  running: 'Running',
  held: 'Held (frozen)',
  superseded: 'Superseded',
}

function formatDeployMarkerTooltip(attempt: DeployAttempt, t: number): string {
  const statusLabel = DEPLOY_MARKER_STATUS_LABEL[attempt.status]
  const started = new Date(t).toLocaleString()
  return `${statusLabel} deploy: ${attempt.image}, started ${started}`
}

function ChartCard({
  appName,
  group,
  range,
  deployMarkers,
}: {
  appName: string
  group: ChartGroupConfig
  range: ResolvedTimeRange
  deployMarkers: ChartMarker[]
}) {
  const primaryQuery = useMetricSeries(appName, group.primary.metric, range)
  // Rules of hooks: this must be called unconditionally on every render
  // regardless of whether `secondary` is set, so `enabled` (not a
  // conditional hook call) is what actually skips the request for
  // single-series cards like CPU.
  const secondaryQuery = useMetricSeries(
    appName,
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
      markers={deployMarkers}
      isLoading={isLoading}
      error={error}
    />
  )
}

export function MetricsDashboard({
  appName,
  deployAttempts,
}: {
  appName: string
  deployAttempts: DeployAttempt[]
}) {
  const [rangeKey, setRangeKey] = useState<TimeRangeKey>(DEFAULT_TIME_RANGE_KEY)
  // Bumped by the Refresh button to recompute "now" without changing
  // rangeKey. Recomputing `range` on every render instead (a bare
  // `resolveTimeRange(rangeKey)` call with no memoization) would mint a
  // new `to` timestamp each render, which is a new query key every
  // render, which is a refetch loop; memoizing on [rangeKey, refreshNonce]
  // keeps the window stable between explicit user actions.
  const [refreshNonce, setRefreshNonce] = useState(0)
  const range = useMemo(
    () => resolveTimeRange(rangeKey),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rangeKey, refreshNonce],
  )
  const deployMarkers = useMemo(
    () => resolveDeployMarkers(deployAttempts, range),
    [deployAttempts, range],
  )
  // Restart count for this range is just "how many samples landed":
  // internal/telemetry.MetricContainerRestartCount writes exactly one
  // sample per real restart (this file's own header comment explains
  // why that's a stat, not a chart), the same "raw sample count is the
  // metric" reading deployMarkers.length already relies on for deploy
  // frequency above.
  const restartCountQuery = useMetricSeries(
    appName,
    'container_restart_count',
    range,
  )
  const restartCount = restartCountQuery.data?.points.length ?? null

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
            As of {range.to.toLocaleTimeString()}. Deploy frequency:{' '}
            {deployMarkers.length} in this range.
            {deployMarkers.length > 0
              ? ' Dashed lines mark real deploy attempts (green succeeded, red failed, gray running); hover a marker for its image tag and start time.'
              : ''}{' '}
            {restartCount !== null
              ? `Restarts: ${restartCount} in this range.`
              : ''}
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
            appName={appName}
            group={group}
            range={range}
            deployMarkers={deployMarkers}
          />
        ))}
      </div>

      <RequestMetricsSection
        appName={appName}
        range={range}
        markers={deployMarkers}
      />
    </section>
  )
}
