import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppVisitors } from '../../queries/appVisitors'
import { VisitorsDetail, VisitorsExportButton } from './VisitorsSection'

const fetchVisitorsExport = vi.hoisted(() => vi.fn())
const saveBlobAs = vi.hoisted(() => vi.fn())
const toastAdd = vi.hoisted(() => vi.fn())
vi.mock('../../queries/appVisitors', async (orig) => ({
  ...(await orig<typeof import('../../queries/appVisitors')>()),
  fetchVisitorsExport,
}))
vi.mock('../../lib/visitorsExport', () => ({ saveBlobAs }))
vi.mock('@/components/ui/toast', () => ({ toast: { add: toastAdd } }))

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

describe('VisitorsDetail with page classes', () => {
  const classed: AppVisitors = {
    ...data,
    requests: 120,
    pages_only: true,
    classes: { page: 4, api: 100, asset: 12, bot: 3, prefetch: 1, other: 0 },
    top_pages: [{ label: '/pricing', count: 4 }],
    top_paths: [{ label: '/api/v1/stats', count: 100 }],
  }

  it('lists page views only and shows the class split', () => {
    render(<VisitorsDetail data={classed} />)
    expect(screen.getByText('/pricing')).toBeTruthy()
    expect(screen.queryByText('/api/v1/stats')).toBeNull()
    expect(screen.getByText('visitors.topPagesCaption')).toBeTruthy()
    expect(screen.getByText('visitors.classes.api 100')).toBeTruthy()
    expect(screen.getByText('visitors.classes.asset 12')).toBeTruthy()
    expect(screen.getByText('visitors.classes.bot 3')).toBeTruthy()
  })

  it('explains an API-only window instead of an empty list', () => {
    render(
      <VisitorsDetail
        data={{
          ...classed,
          top_pages: [],
          classes: {
            page: 0,
            api: 100,
            asset: 12,
            bot: 3,
            prefetch: 0,
            other: 0,
          },
        }}
      />,
    )
    expect(screen.getByText('visitors.noPageViews')).toBeTruthy()
  })

  it('keeps every path while the feature is off', () => {
    render(<VisitorsDetail data={{ ...classed, pages_only: false }} />)
    expect(screen.getByText('/api/v1/stats')).toBeTruthy()
    expect(screen.queryByText('visitors.classSplit')).toBeNull()
    expect(screen.getByText('visitors.topPathsCaption')).toBeTruthy()
  })

  it('falls back to all paths when an older server sends no classes', () => {
    render(
      <VisitorsDetail
        data={{
          ...classed,
          classes: undefined,
          top_pages: undefined,
          pages_only: true,
        }}
      />,
    )
    expect(screen.getByText('/api/v1/stats')).toBeTruthy()
  })
})

describe('VisitorsExportButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('is disabled with nothing to export and explains why', () => {
    render(<VisitorsExportButton app="web" win="24h" requests={0} />)
    const btn = screen.getByRole('button', { name: 'visitors.exportCsv' })
    expect(btn).toHaveProperty('disabled', true)
    expect(btn.getAttribute('title')).toBe('visitors.exportEmpty')
  })

  it('downloads the current window and domain', async () => {
    const blob = new Blob(['a'])
    fetchVisitorsExport.mockResolvedValue({ blob, filename: 'v.csv' })
    render(
      <VisitorsExportButton app="web" win="7d" domain="a.io" requests={5} />,
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'visitors.exportCsv' }),
    )
    await waitFor(() => {
      expect(saveBlobAs).toHaveBeenCalledWith(blob, 'v.csv')
    })
    expect(fetchVisitorsExport).toHaveBeenCalledWith('web', '7d', 'a.io')
  })

  it('toasts the mapped error and re-enables', async () => {
    fetchVisitorsExport.mockRejectedValue(new Error('access log off'))
    render(<VisitorsExportButton app="web" win="24h" requests={5} />)
    const btn = screen.getByRole('button', { name: 'visitors.exportCsv' })
    await userEvent.click(btn)
    await waitFor(() => {
      expect(toastAdd).toHaveBeenCalledWith({
        title: 'access log off',
        type: 'error',
      })
    })
    await waitFor(() => {
      expect(btn).toHaveProperty('disabled', false)
    })
    expect(saveBlobAs).not.toHaveBeenCalled()
  })
})
