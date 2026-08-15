// Shared formatting, series-merging, and marker types for recharts-based
// metric line charts. MetricsDashboard.tsx (per-app) and
// NodeMetricsDashboard.tsx (per-node) both render the same kind of
// small-multiple line chart against a real, live-refreshing time
// series, so the presentational pieces (byte/percent formatting, axis
// tick formatting, merging a primary+secondary series into one row set)
// live here once rather than being copy-pasted a second time when the
// node dashboard was added: exactly the kind of duplication SonarCloud
// has flagged in this codebase before. Data-fetching stays out of this
// file on purpose: the two dashboards query different endpoints
// (queries/metrics.ts vs queries/nodeMetrics.ts) with different cache
// keys, so each owns its own useQuery call and only hands the resolved
// series in here.

import type { MetricSeries } from '../types/metrics'

export type ChartUnit = 'percent' | 'bytes'

export interface ChartRow {
  t: number
  primary?: number
  secondary?: number
}

// A single vertical marker line on a chart, e.g. one deploy attempt on
// the per-app dashboard. Deliberately not shaped around DeployAttempt:
// NodeMetricsDashboard has no per-node deploy history to plot (deploys
// are an app-scoped concept, internal/api/deploy_attempts.go), so it
// simply never produces any markers, rather than this shared chart
// needing to know what a "deploy" is.
export interface ChartMarker {
  key: string
  t: number
  color: string
  tooltip: string
}

// Deliberately not lib/format.ts's formatBytes: that formatter treats 0
// (and negative) as "not set", the right call for an optional resource
// config field (AppOverview's memory limit), but wrong here, where 0
// bytes in a time bucket is a real, meaningful data point, not an
// absent one.
export function formatByteValue(value: number): string {
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

export function formatMetricValue(unit: ChartUnit, value: number): string {
  return unit === 'percent' ? `${value.toFixed(1)}%` : formatByteValue(value)
}

export function formatAxisTick(ms: number, spanMs: number): string {
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

export function mergeMetricSeries(
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

// The most recent primary-series value, formatted, shown next to a
// chart's title as a "current reading" the way Vercel/Grafana
// small-multiple panels pair a stat with a sparkline rather than making
// the reader trace the line to the right edge to find "now".
export function latestReading(rows: ChartRow[], unit: ChartUnit): string | null {
  for (let i = rows.length - 1; i >= 0; i -= 1) {
    const value = rows[i]?.primary
    if (value !== undefined) {
      return formatMetricValue(unit, value)
    }
  }
  return null
}
