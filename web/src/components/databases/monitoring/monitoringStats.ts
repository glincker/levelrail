import type { MetricPoint } from '../../../types/metrics'

export function lastValue(
  points: readonly MetricPoint[] | undefined,
): number | null {
  if (!points || points.length === 0) return null
  return points[points.length - 1]?.value ?? null
}

function mean(points: readonly MetricPoint[]): number {
  return points.reduce((a, p) => a + p.value, 0) / points.length
}

// Percent change of the window average against the previous window average,
// or null when there is nothing honest to compare.
export function deltaVsPrevious(
  points: readonly MetricPoint[] | undefined,
  previous: readonly MetricPoint[] | undefined,
): number | null {
  if (!points?.length || !previous?.length) return null
  const base = mean(previous)
  if (base === 0) return null
  return ((mean(points) - base) / Math.abs(base)) * 100
}

// Polyline coordinates in a 0..100 by 0..24 box, flat line when constant.
export function sparklinePath(
  points: readonly MetricPoint[] | undefined,
  width = 100,
  height = 24,
): string {
  if (!points || points.length < 2) return ''
  const values = points.map((p) => p.value)
  const min = Math.min(...values)
  const span = Math.max(...values) - min
  return values
    .map((v, i) => {
      const x = (i / (values.length - 1)) * width
      const y = span === 0 ? height / 2 : height - ((v - min) / span) * height
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
}
