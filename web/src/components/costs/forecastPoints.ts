import type { CostForecastBand } from '../../types/costExplorer'

export interface ForecastPoint {
  day: number
  actual?: number
  mid?: number
  band?: [number, number]
}

// Cumulative month to date spend, then the projected line and its range to
// the last day of the month.
export function forecastPoints(
  daily: number[],
  daysInMonth: number,
  f: CostForecastBand,
): ForecastPoint[] {
  const points: ForecastPoint[] = []
  let run = 0
  daily.forEach((v, i) => {
    run += v
    points.push({ day: i + 1, actual: run })
  })
  const last = points.length
  if (last === 0 || last >= daysInMonth) return points
  const base = points[last - 1]
  if (base) {
    base.mid = run
    base.band = [run, run]
  }
  for (let d = last + 1; d <= daysInMonth; d++) {
    const k = (d - last) / (daysInMonth - last)
    points.push({
      day: d,
      mid: run + (f.mid - run) * k,
      band: [run + (f.low - run) * k, run + (f.high - run) * k],
    })
  }
  return points
}
