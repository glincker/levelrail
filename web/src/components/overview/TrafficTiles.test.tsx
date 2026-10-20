import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { TrafficStats } from '../../queries/appTraffic'
import { TrafficTiles } from './TrafficTiles'

vi.mock('@/components/kit', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/components/kit')>()),
  SkeletonTile: () => <div data-testid="skeleton" />,
}))

let state: { data?: TrafficStats; isPending: boolean; isError: boolean }
vi.mock('../../queries/appTraffic', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../queries/appTraffic')>()),
  useAppTraffic: () => ({ ...state, refetch: vi.fn() }),
}))

const win = { requests: 600, ratePerSec: 2.5, errorRate: 0.02, p95Ms: 180 }
const stats = (over: Partial<typeof win> = {}): TrafficStats => ({
  hasTraffic: true,
  current: { ...win, ...over },
  previous: { requests: 300, ratePerSec: 1.25, errorRate: 0.02, p95Ms: 180 },
  rateSeries: [1, 2, 3],
  errorSeries: [1, 2, 3],
  p95Series: [1, 2, 3],
})

describe('TrafficTiles', () => {
  it('shows skeleton tiles while loading', () => {
    state = { isPending: true, isError: false }
    render(<TrafficTiles appName="web" url="https://a.io" />)
    expect(screen.getAllByTestId('skeleton')).toHaveLength(3)
  })

  it('shows a friendly empty state with a copy action when there is no traffic', () => {
    state = {
      isPending: false,
      isError: false,
      data: { ...stats(), hasTraffic: false },
    }
    render(<TrafficTiles appName="web" url="https://a.io" />)
    expect(screen.getByText('No traffic yet')).toBeInTheDocument()
    expect(
      screen.getByText(/Send a request to see live numbers/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /copy app url/i }),
    ).toBeInTheDocument()
  })

  it('omits the copy action when the app has no URL', () => {
    state = {
      isPending: false,
      isError: false,
      data: { ...stats(), hasTraffic: false },
    }
    render(<TrafficTiles appName="web" url={null} />)
    expect(screen.queryByRole('button', { name: /copy app url/i })).toBeNull()
  })

  it('renders rate, error share and p95 when populated', () => {
    state = { isPending: false, isError: false, data: stats() }
    render(<TrafficTiles appName="web" url="https://a.io" />)
    expect(screen.getByText('2.50')).toBeInTheDocument()
    expect(screen.getByText('2.0')).toBeInTheDocument()
    expect(screen.getByText('180')).toBeInTheDocument()
  })

  it('shows a delta versus the previous window', () => {
    state = { isPending: false, isError: false, data: stats() }
    render(<TrafficTiles appName="web" url="https://a.io" />)
    expect(screen.getAllByTestId('metric-delta')[0]).toHaveTextContent('100%')
  })

  it('shows an inline error with retry', () => {
    state = { isPending: false, isError: true }
    render(<TrafficTiles appName="web" url="https://a.io" />)
    expect(screen.getByText(/unavailable right now/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })
})
