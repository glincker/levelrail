import { useMemo, useState } from 'react'
import { PulseIcon, InfoIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from './ui/badge'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
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

// Per-app metrics dashboard, TASKS.md 2.4, wired against 2.3's real
// `GET /api/v1/apps/{name}/metrics`. One remaining honest gap against
// the full per-app metrics list the observability phase requires
// without configuration, deliberately not papered over: only the 7
// metrics MetricName covers (types/metrics.ts) are actually collected
// today. Request rate, response time percentiles, error rate, container
// restart count, and build duration are required per-app metrics but
// have no collector behind them yet (see types/metrics.ts's comment for
// exactly why each is missing). NOT_YET_COLLECTED below renders that
// list as a plain, clearly labeled gap, not as empty or fabricated
// charts.
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

const NOT_YET_COLLECTED = [
  'Request rate',
  'Response time percentiles',
  'Error rate',
  'Container restart count',
  'Build duration',
]

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
// visually distinct signal from a routine one, the exact "which deploy
// caused this" framing CLAUDE.md calls out, matching
// DeployAttemptsList.tsx's own succeeded/failed/running badge colors so
// the same status reads the same way in both places.
const DEPLOY_MARKER_COLOR: Record<DeployAttemptStatus, string> = {
  succeeded: '#22c55e',
  failed: '#ef4444',
  running: '#94a3b8',
}

const DEPLOY_MARKER_STATUS_LABEL: Record<DeployAttemptStatus, string> = {
  succeeded: 'Succeeded',
  failed: 'Failed',
  running: 'Running',
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

      <Alert className="mt-4">
        <InfoIcon />
        <AlertTitle>Not yet collected</AlertTitle>
        <AlertDescription>
          <p>
            These are required per-app metrics, but no collector backs them yet:
          </p>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {NOT_YET_COLLECTED.map((label) => (
              <Badge key={label} variant="outline">
                {label}
              </Badge>
            ))}
          </div>
        </AlertDescription>
      </Alert>
    </section>
  )
}
