import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MetricChartCard } from './MetricChartCard'
import type { ChartRow } from '../lib/metricChart'
import type { ResolvedTimeRange } from '../lib/timeRange'

// The recharts SVG this renders has no accessible content of its own
// (see lib/metricChart.ts's chartAccessibleSummary doc comment): these
// tests lock in the role="img" + aria-label text alternative instead of
// only checking the chart renders at all.

const range: ResolvedTimeRange = {
  from: new Date('2026-08-30T00:00:00Z'),
  to: new Date('2026-08-30T01:00:00Z'),
}

function rows(): ChartRow[] {
  return [
    { t: Date.parse('2026-08-30T00:00:00Z'), primary: 10 },
    { t: Date.parse('2026-08-30T00:30:00Z'), primary: 50 },
    { t: Date.parse('2026-08-30T01:00:00Z'), primary: 30 },
  ]
}

describe('MetricChartCard accessible summary', () => {
  it('exposes the chart as an image with a text summary of the data', () => {
    render(
      <MetricChartCard
        title="CPU"
        unit="percent"
        primaryLabel="CPU"
        primaryColor="#0ea5e9"
        rows={rows()}
        range={range}
        isLoading={false}
      />,
    )

    const chart = screen.getByRole('img', {
      name: /CPU chart\. Current 30\.0%, ranging from 10\.0% to 50\.0% over 3 data points\./,
    })
    expect(chart).toBeInTheDocument()
  })

  it('does not render an image role while loading or errored, since there is no chart yet', () => {
    const { rerender } = render(
      <MetricChartCard
        title="CPU"
        unit="percent"
        primaryLabel="CPU"
        primaryColor="#0ea5e9"
        rows={[]}
        range={range}
        isLoading
      />,
    )
    expect(screen.queryByRole('img')).not.toBeInTheDocument()

    rerender(
      <MetricChartCard
        title="CPU"
        unit="percent"
        primaryLabel="CPU"
        primaryColor="#0ea5e9"
        rows={[]}
        range={range}
        isLoading={false}
        error={new Error('boom')}
      />,
    )
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
})
