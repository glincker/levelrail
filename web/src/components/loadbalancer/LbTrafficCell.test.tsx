import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { CommonI18n } from '../../test/commonI18n'
import type { LiveUpstream } from '../../queries/loadBalancerLive'
import { LbTrafficCell } from './LbTrafficCell'

function upstream(patch: Partial<LiveUpstream> = {}): LiveUpstream {
  return {
    id: 'demo#0',
    dial: '10.0.0.1:3000',
    replica: 0,
    weight: 1,
    state: 'healthy',
    healthy: true,
    active_connections: 1,
    fails: 0,
    ...patch,
  }
}

function renderCell(u: LiveUpstream) {
  render(
    <CommonI18n>
      <LbTrafficCell u={u} />
    </CommonI18n>,
  )
}

describe('LbTrafficCell', () => {
  it('shows the empty state when the server sent no traffic', () => {
    renderCell(upstream())
    expect(screen.getByText('No traffic data yet')).toBeTruthy()
    expect(screen.queryByTestId('lb-traffic')).toBeNull()
  })

  it('shows rate, error rate and latency percentiles', () => {
    renderCell(
      upstream({
        traffic: {
          window_seconds: 300,
          requests_per_sec: 12.34,
          error_rate: 0.025,
          p50_ms: 11.6,
          p95_ms: 240,
          rps_series: [10, 12, 14],
        },
      }),
    )
    const cell = screen.getByTestId('lb-traffic')
    expect(cell.textContent).toContain('12.3 req/s')
    expect(cell.textContent).toContain('2.5% errors')
    expect(cell.textContent).toContain('p50 12 ms')
    expect(cell.textContent).toContain('p95 240 ms')
  })

  it('renders an idle upstream without a sparkline', () => {
    renderCell(
      upstream({
        traffic: {
          window_seconds: 60,
          requests_per_sec: 0,
          error_rate: 0,
          p50_ms: 0,
          p95_ms: 0,
          rps_series: [],
        },
      }),
    )
    expect(screen.getByTestId('lb-traffic').textContent).toContain('0 req/s')
  })

  it('formats a zero error rate', () => {
    renderCell(
      upstream({
        traffic: {
          window_seconds: 60,
          requests_per_sec: 3,
          error_rate: 0,
          p50_ms: 4,
          p95_ms: 9,
          rps_series: [],
        },
      }),
    )
    expect(screen.getByTestId('lb-traffic').textContent).toContain('0% errors')
  })
})
