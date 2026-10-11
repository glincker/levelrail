// Builds the vertical marker lines drawn on every metric chart: one per
// deploy attempt and one per bucket of container restarts.

import type { DeployAttempt, DeployAttemptStatus } from '../types/deployAttempt'
import type { MetricPoint } from '../types/metrics'
import type { ChartMarker } from './metricChart'
import { chartPalette } from './chartPalette'
import type { ResolvedTimeRange } from './timeRange'

export const DEPLOY_MARKER_COLOR: Record<DeployAttemptStatus, string> = {
  succeeded: chartPalette.success,
  failed: chartPalette.danger,
  running: chartPalette.neutral,
  held: chartPalette.warning,
  superseded: chartPalette.muted,
  queued: chartPalette.muted,
  canceled: chartPalette.muted,
}

// A burst of restarts collapses into one marker per 1/RESTART_BUCKETS of the
// visible range so a crashloop does not paint hundreds of lines.
const RESTART_BUCKETS = 80

export function deployMarkers(
  attempts: readonly DeployAttempt[],
  range: ResolvedTimeRange,
  tooltip: (attempt: DeployAttempt, t: number) => string,
): ChartMarker[] {
  const out: ChartMarker[] = []
  for (const attempt of attempts) {
    const t = Date.parse(attempt.started_at)
    if (Number.isNaN(t) || t < range.from.getTime() || t > range.to.getTime()) {
      continue
    }
    out.push({
      key: attempt.id,
      t,
      color: DEPLOY_MARKER_COLOR[attempt.status],
      tooltip: tooltip(attempt, t),
      kind: 'deploy',
    })
  }
  return out.sort((a, b) => a.t - b.t)
}

export interface RestartBucket {
  t: number
  count: number
}

export function bucketRestarts(
  points: readonly MetricPoint[],
  range: ResolvedTimeRange,
): RestartBucket[] {
  const from = range.from.getTime()
  const span = Math.max(1, range.to.getTime() - from)
  const size = span / RESTART_BUCKETS
  const buckets = new Map<number, RestartBucket>()
  for (const p of points) {
    const t = Date.parse(p.timestamp)
    if (Number.isNaN(t) || t < from || t > range.to.getTime()) {
      continue
    }
    const idx = Math.min(RESTART_BUCKETS - 1, Math.floor((t - from) / size))
    const b = buckets.get(idx)
    if (b) {
      b.count += 1
    } else {
      buckets.set(idx, { t, count: 1 })
    }
  }
  return Array.from(buckets.values()).sort((a, b) => a.t - b.t)
}

export function restartMarkers(
  buckets: readonly RestartBucket[],
  tooltip: (count: number, t: number) => string,
): ChartMarker[] {
  return buckets.map((b) => ({
    key: `restart-${b.t}`,
    t: b.t,
    color: chartPalette.warning,
    tooltip: tooltip(b.count, b.t),
    kind: 'restart',
  }))
}
