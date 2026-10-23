import type { MetricSeries } from '../../../types/metrics'

export type Headroom = 'unknown' | 'tight' | 'ok' | 'ample'

export function latestValue(series: MetricSeries | undefined): number | null {
  const pts = series?.points ?? []
  return pts.length > 0 ? (pts[pts.length - 1]?.value ?? null) : null
}

export function peakValue(series: MetricSeries | undefined): number | null {
  const pts = series?.points ?? []
  if (pts.length === 0) return null
  return pts.reduce((max, p) => Math.max(max, p.value), 0)
}

// Guidance only, never gates a save; tunable per build.
function envRatio(raw: string | undefined, fallback: number): number {
  const n = Number(raw)
  return raw && Number.isFinite(n) && n > 0 && n < 1 ? n : fallback
}

const TIGHT_RATIO = envRatio(
  import.meta.env.VITE_DB_SIZING_TIGHT_RATIO as string | undefined,
  0.85,
)
const AMPLE_RATIO = envRatio(
  import.meta.env.VITE_DB_SIZING_AMPLE_RATIO as string | undefined,
  0.3,
)

export function headroom(peak: number | null, limit: number): Headroom {
  if (peak === null || limit <= 0) return 'unknown'
  const ratio = peak / limit
  if (ratio >= TIGHT_RATIO) return 'tight'
  if (ratio <= AMPLE_RATIO) return 'ample'
  return 'ok'
}

export interface ResourceChange {
  memoryBytes: number
  nanoCpus: number
}

export type ImpactKind = 'none' | 'stopped' | 'live'

export function impactKind(
  suspended: boolean,
  before: ResourceChange,
  after: ResourceChange,
): ImpactKind {
  const changed =
    before.memoryBytes !== after.memoryBytes ||
    before.nanoCpus !== after.nanoCpus
  if (!changed) return 'none'
  return suspended ? 'stopped' : 'live'
}

export function shrinksBelowPeak(
  peakMemoryBytes: number | null,
  newMemoryBytes: number,
): boolean {
  return (
    peakMemoryBytes !== null &&
    newMemoryBytes > 0 &&
    newMemoryBytes < peakMemoryBytes
  )
}
