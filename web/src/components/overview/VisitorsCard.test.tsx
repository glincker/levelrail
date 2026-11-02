import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AppVisitors } from '../../queries/appVisitors'
import { VisitorsBody } from './VisitorsCard'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      typeof o?.when === 'string' ? `${k}:${o.when}` : k,
  }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

const base: AppVisitors = {
  app: 'web',
  window: '24h',
  since: '2026-11-01T00:00:00Z',
  generated_at: '2026-11-02T00:00:00Z',
  requests: 100,
  visitors: 12,
  status: { '2xx': 90, '3xx': 0, '4xx': 5, '5xx': 5 },
  top_paths: [{ label: '/', count: 60 }],
  top_referrers: [],
  series: [{ start: '2026-11-01T00:00:00Z', requests: 3, visitors: 1 }],
  domains: [
    { domain: 'a.io', requests: 60, visitors: 7 },
    { domain: 'b.io', requests: 40, visitors: 5 },
  ],
  salt_period: '24h',
}

describe('VisitorsBody', () => {
  it('shows numbers, status split and per-domain row', () => {
    render(<VisitorsBody data={base} />)
    expect(screen.getByText('100')).toBeTruthy()
    expect(screen.getByText('12')).toBeTruthy()
    expect(screen.getByText('2xx 90%')).toBeTruthy()
    expect(screen.getByText('a.io')).toBeTruthy()
  })

  it('notes partial data when the ring starts after the window', () => {
    render(
      <VisitorsBody
        data={{ ...base, oldest_record: '2026-11-01T12:00:00Z' }}
      />,
    )
    expect(screen.getByText(/visitors.partial/)).toBeTruthy()
  })

  it('shows the empty state with no requests', () => {
    render(<VisitorsBody data={{ ...base, requests: 0 }} />)
    expect(screen.getByText('visitors.empty')).toBeTruthy()
  })
})
