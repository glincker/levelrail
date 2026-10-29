import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/apiError'
import { TopAppsWidget } from './TopAppsWidget'
import { renderWidget } from './testKit'

vi.mock('@tanstack/react-router', async () =>
  (await import('./testKit')).routerMock(),
)
const state = vi.hoisted(() => ({ q: {} }))
vi.mock('@/queries/appsMetrics', () => ({ useAppsMetrics: () => state.q }))

const base = { has_traffic: true, error_rate_5xx: 0, p95_ms: 1, spark: [] }

describe('TopAppsWidget', () => {
  it('handles telemetry not configured', () => {
    state.q = { error: new ApiError(501, 'x') }
    renderWidget(<TopAppsWidget />)
    expect(screen.getByText('Telemetry is not configured.')).toBeInTheDocument()
  })

  it('shows an error state', () => {
    state.q = { error: new Error('x') }
    renderWidget(<TopAppsWidget />)
    expect(screen.getByText('Could not load this data.')).toBeInTheDocument()
  })

  it('ranks by CPU then switches to requests', () => {
    state.q = {
      data: [
        { ...base, name: 'low', cpu_percent: 1, rate_per_sec: 9 },
        { ...base, name: 'high', cpu_percent: 50, rate_per_sec: 1 },
      ],
    }
    renderWidget(<TopAppsWidget />)
    let links = screen.getAllByRole('link')
    expect(links[0]).toHaveTextContent('high')
    expect(links[0]).toHaveTextContent('50.0%')
    fireEvent.click(screen.getByRole('button', { name: 'Requests' }))
    links = screen.getAllByRole('link')
    expect(links[0]).toHaveTextContent('low')
  })

  it('shows an empty state when nothing ranks', () => {
    state.q = { data: [] }
    renderWidget(<TopAppsWidget />)
    expect(screen.getByText('Nothing to rank yet.')).toBeInTheDocument()
  })
})
