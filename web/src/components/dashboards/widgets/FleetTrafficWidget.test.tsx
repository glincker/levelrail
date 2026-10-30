import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/apiError'
import { FleetTrafficWidget } from './FleetTrafficWidget'
import { renderWidget } from './testKit'

const state = vi.hoisted(() => ({ q: {} }))
vi.mock('@/queries/appsMetrics', () => ({ useAppsMetrics: () => state.q }))

const row = (rate: number, p95: number) => ({
  name: 'a',
  has_traffic: true,
  rate_per_sec: rate,
  error_rate_5xx: 0.1,
  p95_ms: p95,
  spark: [1, 2, 3],
})

describe('FleetTrafficWidget', () => {
  it('says telemetry is not configured on 501, never zero', () => {
    state.q = { error: new ApiError(501, 'x') }
    renderWidget(<FleetTrafficWidget />)
    expect(screen.getByText('Telemetry is not configured.')).toBeInTheDocument()
  })

  it('shows an error state', () => {
    state.q = { error: new Error('x') }
    renderWidget(<FleetTrafficWidget />)
    expect(screen.getByText('Could not load this data.')).toBeInTheDocument()
  })

  it('shows an empty state without traffic', () => {
    state.q = { data: [] }
    renderWidget(<FleetTrafficWidget />)
    expect(
      screen.getByText('No requests in the last hour.'),
    ).toBeInTheDocument()
  })

  it('shows requests, errors and p95', () => {
    state.q = { data: [row(1, 120)] }
    renderWidget(<FleetTrafficWidget />)
    expect(screen.getByText('3,600')).toBeInTheDocument()
    expect(screen.getByText('10.00%')).toBeInTheDocument()
    expect(screen.getByText('120 ms')).toBeInTheDocument()
  })
})
