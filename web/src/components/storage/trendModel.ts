import type { DiskSample } from '../../queries/diskHistory'

export const DAY_MS = 86_400_000
export const MIN_FORECAST_DAYS = 7
const RANGE_CHOICES = [7, 30, 90] as const

export interface TrendPoint {
  t: number
  used: number
  free: number
  total: number
  images: number
  containers: number
  volumes: number
  buildCache: number
}

export interface TrendScale {
  x0: number
  x1: number
  yMax: number
}

export function usedBytes(s: Pick<DiskSample, 'free_bytes' | 'total_bytes'>) {
  return Math.max(0, s.total_bytes - s.free_bytes)
}

export function toPoints(samples: DiskSample[]): TrendPoint[] {
  return samples
    .map((s) => ({
      t: Date.parse(s.at),
      used: usedBytes(s),
      free: s.free_bytes,
      total: s.total_bytes,
      images: s.images_bytes,
      containers: s.containers_bytes,
      volumes: s.volumes_bytes,
      buildCache: s.build_cache_bytes,
    }))
    .filter((p) => Number.isFinite(p.t))
    .sort((a, b) => a.t - b.t)
}

export function distinctDays(points: TrendPoint[]): number {
  return new Set(points.map((p) => new Date(p.t).toISOString().slice(0, 10)))
    .size
}

// Range switch offers 7, 30 and 90 days, never more than the server keeps.
export function rangeOptions(retentionDays: number): number[] {
  const ok = RANGE_CHOICES.filter((d) => d <= retentionDays)
  return ok.length > 0 ? ok : [Math.max(1, retentionDays)]
}

export function defaultRange(options: number[], preferred = 30): number {
  if (options.includes(preferred)) return preferred
  return options[options.length - 1] ?? preferred
}

export interface Projection {
  from: TrendPoint
  to: { t: number; used: number }
}

// Straight line from the latest sample to the moment used reaches total.
export function projection(
  points: TrendPoint[],
  daysUntilFull: number | null,
): Projection | null {
  const last = points[points.length - 1]
  if (!last || daysUntilFull === null || !(daysUntilFull > 0)) return null
  return {
    from: last,
    to: { t: last.t + daysUntilFull * DAY_MS, used: last.total },
  }
}

export function buildScale(
  points: TrendPoint[],
  proj: Projection | null,
): TrendScale {
  const first = points[0]?.t ?? 0
  const lastT = points[points.length - 1]?.t ?? first + DAY_MS
  const x1 = Math.max(proj?.to.t ?? lastT, first + DAY_MS)
  const peak = Math.max(...points.map((p) => p.used), 1)
  const yMax = proj ? proj.to.used : peak * 1.1
  return { x0: first, x1, yMax: Math.max(yMax, 1) }
}

export function xAt(scale: TrendScale, t: number, width: number): number {
  return ((t - scale.x0) / (scale.x1 - scale.x0)) * width
}

export function yAt(scale: TrendScale, v: number, height: number): number {
  return height - (v / scale.yMax) * height
}

export function linePath(
  points: Array<{ t: number; used: number }>,
  scale: TrendScale,
  w: number,
  h: number,
): string {
  return points
    .map(
      (p, i) =>
        `${i === 0 ? 'M' : 'L'}${xAt(scale, p.t, w).toFixed(1)} ${yAt(scale, p.used, h).toFixed(1)}`,
    )
    .join(' ')
}

export function areaPath(
  points: TrendPoint[],
  scale: TrendScale,
  w: number,
  h: number,
): string {
  const last = points[points.length - 1]
  const first = points[0]
  if (!first || !last) return ''
  return `${linePath(points, scale, w, h)} L${xAt(scale, last.t, w).toFixed(1)} ${h} L${xAt(scale, first.t, w).toFixed(1)} ${h} Z`
}

// Nearest sample to a pixel x, so hover and arrow keys share one lookup.
export function nearestIndex(
  points: TrendPoint[],
  scale: TrendScale,
  px: number,
  w: number,
): number {
  if (points.length === 0) return -1
  let best = 0
  let bestD = Infinity
  points.forEach((p, i) => {
    const d = Math.abs(xAt(scale, p.t, w) - px)
    if (d < bestD) {
      bestD = d
      best = i
    }
  })
  return best
}
