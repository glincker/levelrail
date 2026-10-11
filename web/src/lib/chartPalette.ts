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
