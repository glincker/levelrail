import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  deriveMonitorState,
  uptimePercent,
  suggestMonitorName,
} from '@/lib/monitors'
import { MonitorsList } from './MonitorsSection'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Monitor, MonitorResults } from '../../types/monitor'

const results = (oks: boolean[]): MonitorResults => ({
  monitor_id: 'm1',
  raw: oks.map((ok, i) => ({ at: i + 1, ok, latency_ms: 10 })),
  rollup: [
    { bucket: 0, total: 4, failed: 1, avg_latency_ms: 5, max_latency_ms: 9 },
  ],
})

const mon: Monitor = {
  id: 'm1',
  name: 'Site',
  kind: 'http',
  interval_seconds: 60,
  timeout_ms: 5000,
  enabled: true,
  url: 'https://example.com',
  created_at: '2026-01-01T00:00:00Z',
}

describe('monitor helpers', () => {
  it('derives state from the newest result', () => {
    expect(deriveMonitorState(mon, results([true, false]))).toBe('down')
    expect(deriveMonitorState(mon, results([false, true]))).toBe('up')
    expect(deriveMonitorState(mon, undefined)).toBe('unknown')
    expect(deriveMonitorState({ enabled: false }, results([true]))).toBe(
      'paused',
    )
  })
  it('prefers the confirmed state and falls back when absent', () => {
    expect(
      deriveMonitorState({ enabled: true, state: 'suspect' }, results([true])),
    ).toBe('suspect')
    expect(
      deriveMonitorState({ enabled: true, state: 'recovering' }, undefined),
    ).toBe('recovering')
    expect(deriveMonitorState({ enabled: true }, results([false]))).toBe('down')
  })
  it('computes uptime and name suggestion', () => {
    expect(uptimePercent(results([true]))).toBe(75)
    expect(suggestMonitorName('https://api.example.com/x')).toBe(
      'api.example.com',
    )
  })
})

describe('MonitorsList', () => {
  it('shows the empty hint', () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MonitorsList monitors={[]} onDelete={() => undefined} />
      </QueryClientProvider>,
    )
    expect(screen.getByText(/empty|No monitors/i)).toBeTruthy()
  })
})
