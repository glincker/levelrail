import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import TrafficSeriesChart from './TrafficSeriesChart'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

const point = (requests: number, errors: number) => ({
  start: '2026-01-01T00:00:00Z',
  requests,
  pages: requests,
  assets: 0,
  errors,
})

describe('TrafficSeriesChart', () => {
  it('shows the empty state when nothing was served', () => {
    render(<TrafficSeriesChart series={[point(0, 0), point(0, 0)]} />)
    expect(screen.getByText('traffic.noSeries')).toBeTruthy()
  })

  it('draws rows and only shows errors when there are some', () => {
    const { rerender } = render(
      <TrafficSeriesChart series={[point(2, 0), point(3, 0)]} />,
    )
    expect(screen.queryByText('traffic.series.errors')).toBeNull()
    rerender(<TrafficSeriesChart series={[point(2, 1), point(3, 0)]} />)
    expect(screen.getByText('traffic.series.errors')).toBeTruthy()
  })
})
