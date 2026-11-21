export const METRIC_NAMES_DEFAULT = 'CPUUtilization,NetworkIn'
export const METRIC_HOURS_DEFAULT = 3
export const LOG_LIMIT_DEFAULT = 50
export const LOG_HOURS_DEFAULT = 1

const SPARK_W = 120
const SPARK_H = 28

export function snapshotTarget(target: string): {
  instance_id?: string
  volume_id?: string
} {
  const v = target.trim()
  if (v.startsWith('vol-')) return { volume_id: v }
  return v === '' ? {} : { instance_id: v }
}

export function idleTotalUsd(
  rows: ReadonlyArray<{ est_monthly_usd: number }>,
): number {
  return rows.reduce((sum, r) => sum + (r.est_monthly_usd || 0), 0)
}

// Polyline points scaled into a fixed viewBox so the SVG needs no inline style.
export function sparkPoints(values: readonly number[]): string {
  if (values.length === 0) return ''
  const min = Math.min(...values)
  const span = Math.max(...values) - min || 1
  const step = values.length > 1 ? SPARK_W / (values.length - 1) : 0
  return values
    .map((v, i) => {
      const y = SPARK_H - ((v - min) / span) * SPARK_H
      return `${(i * step).toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
}

export const SPARK_VIEWBOX = `0 0 ${SPARK_W} ${SPARK_H}`
