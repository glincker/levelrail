import type { UsageDimension, UsageGroup, UsageReport } from '../types/usage'

export function formatMoney(value: number, currency: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
      maximumFractionDigits: 2,
    }).format(value)
  } catch {
    return `${value.toFixed(2)} ${currency}`
  }
}

export function formatDeltaPct(pct: number | undefined): string {
  if (pct === undefined) return '-'
  const rounded = Math.round(pct)
  return `${rounded > 0 ? '+' : ''}${rounded}%`
}

// Display name of a group: a resource is its own name, the rest are looked
// up in the report's labels with the raw key as the fallback.
export function groupName(
  report: Pick<UsageReport, 'labels'>,
  dim: UsageDimension,
  g: UsageGroup,
  unassigned: string,
): string {
  if (dim === 'resource') return g.name ?? g.key
  if (dim === 'tag') return g.key === 'untagged' ? unassigned : g.key
  if (g.key === 'unassigned') return unassigned
  return report.labels[`${dim}:${g.key}`] ?? g.key
}

// Points of a sparkline in a 0..1 box; a flat or empty series sits mid line.
export function sparklinePoints(daily: number[]): string {
  if (daily.length < 2) return ''
  const max = Math.max(...daily)
  const min = Math.min(...daily)
  const span = max - min
  return daily
    .map((v, i) => {
      const x = (i / (daily.length - 1)) * 100
      const y = span === 0 ? 12 : 22 - ((v - min) / span) * 20
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
}

export interface StackedSeries {
  key: string
  label: string
  values: number[]
}

// Top projects as their own series, the rest folded into one, so the chart
// stays readable with many projects.
export function stackSeries(
  report: UsageReport,
  maxSeries: number,
  labels: { other: string; unassigned: string },
): StackedSeries[] {
  const days = report.days.length
  const top = report.projects.slice(0, maxSeries)
  const out: StackedSeries[] = top.map((g) => ({
    key: g.key,
    label: groupName(report, 'project', g, labels.unassigned),
    values: g.daily,
  }))
  const rest = report.projects.slice(maxSeries)
  if (rest.length > 0) {
    const values = new Array<number>(days).fill(0)
    for (const g of rest) {
      g.daily.forEach((v, i) => {
        values[i] = (values[i] ?? 0) + v
      })
    }
    out.push({ key: '__other', label: labels.other, values })
  }
  return out
}
