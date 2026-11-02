import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AppVisitors } from '../../queries/appVisitors'
import { VisitorsDetail } from './VisitorsSection'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

const data: AppVisitors = {
  app: 'web',
  window: '30d',
  since: '2026-10-03T00:00:00Z',
  generated_at: '2026-11-02T00:00:00Z',
  requests: 100,
  visitors: 12,
  status: { '2xx': 90, '3xx': 0, '4xx': 5, '5xx': 5 },
  top_paths: Array.from({ length: 12 }, (_, i) => ({
    label: `/p${i}`,
    count: 12 - i,
  })),
  top_referrers: [],
  series: [{ start: '2026-11-01T00:00:00Z', requests: 3, visitors: 1 }],
  domains: [],
  salt_period: '24h',
}

describe('VisitorsDetail', () => {
  it('caps top pages at ten rows and shows the status split', () => {
    render(<VisitorsDetail data={data} />)
    expect(screen.getByText('/p9')).toBeTruthy()
    expect(screen.queryByText('/p10')).toBeNull()
    expect(screen.getAllByText('2xx 90%').length).toBeGreaterThan(0)
  })

  it('shows the empty state with no requests', () => {
    render(<VisitorsDetail data={{ ...data, requests: 0 }} />)
    expect(screen.getByText('visitors.empty')).toBeTruthy()
  })
})
