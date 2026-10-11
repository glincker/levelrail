// CSS variables, not hex: recharts writes them to SVG presentation attributes,
// so light and dark values come from index.css and never drift per chart.
export const chartSeries = [
  'var(--chart-series-1)',
  'var(--chart-series-2)',
  'var(--chart-series-3)',
  'var(--chart-series-4)',
  'var(--chart-series-5)',
  'var(--chart-series-6)',
] as const

export const chartPalette = {
  primary: chartSeries[0],
  warning: chartSeries[1],
  success: chartSeries[2],
  neutral: chartSeries[3],
  danger: 'var(--chart-danger)',
  muted: 'var(--chart-muted)',
  selection: 'var(--tone-accent-solid)',
} as const

export const chartDanger = 'var(--chart-danger)'
export const chartMuted = 'var(--chart-muted)'

// Stable slot per metric kind so CPU is always series 1 across charts.
export const chartSlot = {
  cpu: chartSeries[0],
  memory: chartSeries[1],
  network: chartSeries[2],
  disk: chartSeries[3],
  requests: chartSeries[4],
  latency: chartSeries[5],
  errors: chartDanger,
} as const

export function seriesColor(index: number): string {
  const n = chartSeries.length
  return chartSeries[((index % n) + n) % n] ?? chartSeries[0]
}
