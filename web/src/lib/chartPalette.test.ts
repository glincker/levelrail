import { describe, expect, it } from 'vitest'
import { chartSeries, chartSlot, seriesColor } from './chartPalette'

describe('chartPalette', () => {
  it('uses theme variables only', () => {
    for (const c of chartSeries) expect(c).toMatch(/^var\(--chart-series-\d\)$/)
  })
  it('keeps cpu on series 1', () => {
    expect(chartSlot.cpu).toBe('var(--chart-series-1)')
  })
  it('wraps indices', () => {
    expect(seriesColor(6)).toBe(chartSeries[0])
    expect(seriesColor(-1)).toBe(chartSeries[5])
  })
})
