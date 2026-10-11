// Pure helpers that shape server series into chart rows: previous period
// alignment and a render bound for long ranges.

import type { ChartRow } from './metricChart'

// The server downsamples to at most this many points per series; the client
// applies the same bound again (capRows) so a chart never renders more.
export const MAX_CHART_POINTS = 600

interface TimedValue {
  timestamp: string
  value: number
}

// attachPrevious merges the previous period (timestamps already shifted onto
// the current window by the server) into rows as `previous`. A previous point
// with no current row at the same instant gets its own row, so the dashed
// line never loses data.
export function attachPrevious(
  rows: ChartRow[],
  previous: readonly TimedValue[] | undefined,
): ChartRow[] {
  if (!previous || previous.length === 0) {
    return rows
  }
  const byT = new Map<number, ChartRow>()
  for (const row of rows) {
    byT.set(row.t, { ...row })
  }
  for (const p of previous) {
    const t = Date.parse(p.timestamp)
    if (Number.isNaN(t)) {
      continue
    }
    const existing = byT.get(t)
    if (existing) {
      existing.previous = p.value
    } else {
      byT.set(t, { t, previous: p.value })
    }
  }
  return Array.from(byT.values()).sort((a, b) => a.t - b.t)
}

// capRows thins rows to at most `max` by keeping each bucket's lowest and
// highest primary value, so a one-sample spike survives the thinning.
export function capRows(
  rows: ChartRow[],
  max: number = MAX_CHART_POINTS,
): ChartRow[] {
  if (max < 2 || rows.length <= max) {
    return rows
  }
  const buckets = Math.floor(max / 2)
  const size = Math.ceil(rows.length / buckets)
  const out: ChartRow[] = []
  for (let start = 0; start < rows.length; start += size) {
    const slice = rows.slice(start, start + size)
    let lo = slice[0]
    let hi = slice[0]
    for (const r of slice) {
      const v = r.primary ?? r.previous ?? 0
      if (v < (lo?.primary ?? lo?.previous ?? 0)) {
        lo = r
      }
      if (v > (hi?.primary ?? hi?.previous ?? 0)) {
        hi = r
      }
    }
    if (lo && hi) {
      if (lo === hi) {
        out.push(lo)
      } else if (lo.t < hi.t) {
        out.push(lo, hi)
      } else {
        out.push(hi, lo)
      }
    }
  }
  return out
}
