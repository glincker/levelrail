import { useMemo, useState } from 'react'
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import {
  PulseIcon,
  InfoIcon,
  ArrowsClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { useMetricSeries } from '../queries/metrics'
import type { MetricName, MetricSeries } from '../types/metrics'
import type { ReconcileCondition } from '../types/deploy'
import {
  DEFAULT_TIME_RANGE_KEY,
  TIME_RANGE_PRESETS,
  resolveTimeRange,
  type ResolvedTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'

// Per-app metrics dashboard, TASKS.md 2.4, wired against 2.3's real
// `GET /api/v1/apps/{name}/metrics`. Two honest gaps against CLAUDE.md 6
// and 4.8's full metrics list, deliberately not papered over:
//
// 1. Only the 7 metrics MetricName covers (types/metrics.ts) are
//    actually collected today. Request rate, response time percentiles,
//    error rate, container restart count, build duration, and deploy
//    frequency are named in CLAUDE.md 6 but have no collector behind
//    them yet (see types/metrics.ts's comment for exactly why each is
//    missing). NOT_YET_COLLECTED below renders that list as a plain,
//    clearly labeled gap, not as empty or fabricated charts.
// 2. "Deploy markers overlaid on metric charts": GET /api/v1/apps/
//    {name}/deploys (internal/api/deploys.go's handleDeployHistory) only
//    ever returns the *current* reconcile condition per (controller,
//    type) pair, there is no attempt-by-attempt deploy history to plot
//    as multiple markers across a time range (see
//    internal/api/deploys.go and queries/deploys.ts's own doc comments).
//    The honest, real thing available is the app's current "Ready"
//    condition's LastTransitionTime: at most one marker, shown only
//    when it falls inside the visible range. resolveDeployMarker below
//    does exactly that and nothing more. This is a partial
//    approximation, not a real deploy-history feature.

const NOT_YET_COLLECTED = [
  'Request rate',
  'Response time percentiles',
  'Error rate',
  'Container restart count',
  'Build duration',
  'Deploy frequency',
]

// internal/reconcile/application.Controller only ever emits a "Ready"
// condition type (see internal/reconcile/application/controller.go),
// matching applicationControllerName's scope in internal/api/deploys.go.
const READY_CONDITION_TYPE = 'Ready'

type ChartUnit = 'percent' | 'bytes'

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

// Deliberately not lib/format.ts's formatBytes: that formatter treats 0
// (and negative) as "not set", the right call for an optional resource
// config field (AppOverview's memory limit), but wrong here, where 0
// received bytes in a time bucket is a real, meaningful data point, not
// an absent one.
function formatByteValue(value: number): string {
  if (!Number.isFinite(value)) {
    return '-'
  }
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let v = Math.abs(value)
  let unitIndex = 0
  while (v >= 1024 && unitIndex < units.length - 1) {
    v /= 1024
    unitIndex += 1
  }
  const sign = value < 0 ? '-' : ''
  const decimals = unitIndex > 0 && v < 10 ? 1 : 0
  return `${sign}${v.toFixed(decimals)} ${units[unitIndex]}`
}

function formatMetricValue(unit: ChartUnit, value: number): string {
  return unit === 'percent' ? `${value.toFixed(1)}%` : formatByteValue(value)
}

function formatAxisTick(ms: number, spanMs: number): string {
  const date = new Date(ms)
  // Longer than ~36h: bare hour:minute stops being enough to tell points
  // apart across days, so a short date prefix is added.
  if (spanMs > 36 * 60 * 60 * 1000) {
    return date.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
    })
  }
  return date.toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  })
}

interface ChartRow {
  t: number
  primary?: number
  secondary?: number
}

function mergeMetricSeries(
  primary: MetricSeries | undefined,
  secondary: MetricSeries | undefined,
): ChartRow[] {
  const rows = new Map<number, ChartRow>()
  for (const point of primary?.points ?? []) {
    const t = Date.parse(point.timestamp)
    if (Number.isNaN(t)) {
      continue
    }
    rows.set(t, { ...rows.get(t), t, primary: point.value })
  }
  for (const point of secondary?.points ?? []) {
    const t = Date.parse(point.timestamp)
    if (Number.isNaN(t)) {
      continue
    }
    rows.set(t, { ...rows.get(t), t, secondary: point.value })
  }
  return Array.from(rows.values()).sort((a, b) => a.t - b.t)
}

// resolveDeployMarker returns the current "Ready" condition's
// LastTransitionTime as an epoch-ms marker, but only when it falls
// inside the chart's visible range; otherwise there is nothing honest to
// draw. See this file's header comment for why this is a single-point
// approximation, not real deploy history.
function resolveDeployMarker(
  conditions: ReconcileCondition[],
  range: ResolvedTimeRange,
): number | null {
  const ready = conditions.find((c) => c.Type === READY_CONDITION_TYPE)
  if (!ready) {
    return null
  }
  const t = Date.parse(ready.LastTransitionTime)
  if (Number.isNaN(t)) {
    return null
  }
  if (t < range.from.getTime() || t > range.to.getTime()) {
    return null
  }
  return t
}

// The most recent value across both series, shown next to the chart
// title as a "current reading" the way Vercel/Grafana small-multiple
// panels pair a stat with a sparkline rather than making the reader
// trace the line to the right edge to find "now".
function latestReading(
  rows: ChartRow[],
  group: ChartGroupConfig,
): string | null {
  for (let i = rows.length - 1; i >= 0; i -= 1) {
    const row = rows[i]
    const value = row?.primary
    if (value !== undefined) {
      return formatMetricValue(group.unit, value)
    }
  }
  return null
}

function ChartCard({
  appName,
  group,
  range,
  deployMarkerMs,
}: {
  appName: string
  group: ChartGroupConfig
  range: ResolvedTimeRange
  deployMarkerMs: number | null
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

  const spanMs = range.to.getTime() - range.from.getTime()
  const current = isLoading || error ? null : latestReading(rows, group)

  return (
    <section className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-center justify-between gap-2">
        <h3 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
          <span
            className="size-2 shrink-0 rounded-full"
            style={{ backgroundColor: group.primary.color }}
            aria-hidden="true"
          />
          {group.title}
        </h3>
        {current !== null ? (
          <span className="font-mono text-sm font-medium text-foreground">
            {current}
          </span>
        ) : null}
      </div>
      {isLoading ? (
        <p className="mt-6 text-sm text-muted-foreground">Loading...</p>
      ) : error ? (
        <p className="mt-6 text-sm text-destructive">{error.message}</p>
      ) : rows.length === 0 ? (
        <p className="mt-6 text-sm text-muted-foreground">
          No data in this range.
        </p>
      ) : (
        <div className="mt-2 h-56">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart
              data={rows}
              margin={{ top: 4, right: 8, left: 0, bottom: 0 }}
            >
              <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
              <XAxis
                dataKey="t"
                type="number"
                domain={['dataMin', 'dataMax']}
                tickFormatter={(t: number) => formatAxisTick(t, spanMs)}
                tick={{ fontSize: 11 }}
                stroke="currentColor"
                className="text-muted-foreground"
              />
              <YAxis
                tickFormatter={(v: number) => formatMetricValue(group.unit, v)}
                tick={{ fontSize: 11 }}
                width={64}
                stroke="currentColor"
                className="text-muted-foreground"
              />
              <Tooltip
                labelFormatter={(label) =>
                  new Date(Number(label)).toLocaleString()
                }
                formatter={(value) =>
                  formatMetricValue(group.unit, Number(value))
                }
              />
              {group.secondary ? (
                <Legend wrapperStyle={{ fontSize: 11 }} />
              ) : null}
              {deployMarkerMs !== null ? (
                <ReferenceLine
                  x={deployMarkerMs}
                  stroke="#ef4444"
                  strokeDasharray="4 4"
                  label={{
                    value: 'Deploy',
                    position: 'insideTopRight',
                    fontSize: 10,
                    fill: '#ef4444',
                  }}
                />
              ) : null}
              <Line
                type="monotone"
                dataKey="primary"
                name={group.primary.label}
                stroke={group.primary.color}
                dot={false}
                strokeWidth={1.5}
                isAnimationActive={false}
                connectNulls
              />
              {group.secondary ? (
                <Line
                  type="monotone"
                  dataKey="secondary"
                  name={group.secondary.label}
                  stroke={group.secondary.color}
                  dot={false}
                  strokeWidth={1.5}
                  isAnimationActive={false}
                  connectNulls
                />
              ) : null}
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </section>
  )
}

export function MetricsDashboard({
  appName,
  conditions,
}: {
  appName: string
  conditions: ReconcileCondition[]
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
  const deployMarkerMs = useMemo(
    () => resolveDeployMarker(conditions, range),
    [conditions, range],
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
            {deployMarkerMs !== null
              ? " Red dashed line marks the current reconcile's last transition, not a full deploy history (see code comment)."
              : ''}
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
        {CHART_GROUPS.map((group) => (
          <ChartCard
            key={group.title}
            appName={appName}
            group={group}
            range={range}
            deployMarkerMs={deployMarkerMs}
          />
        ))}
      </div>

      <Alert className="mt-4">
        <InfoIcon />
        <AlertTitle>Not yet collected</AlertTitle>
        <AlertDescription>
          <p>
            CLAUDE.md 6 also names these as required per-app metrics, but no
            collector backs them yet:
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
