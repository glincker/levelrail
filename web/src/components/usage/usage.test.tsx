import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/usage.json'
import {
  formatDeltaPct,
  sparklinePoints,
  stackSeries,
} from '../../lib/usageFormat'
import { AppCostChip } from './AppCostChip'
import { UsageTable } from './UsageTable'
import type { UsageGroup, UsageReport } from '../../types/usage'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['usage'],
  defaultNS: 'usage',
  resources: { en: { usage: en } },
  interpolation: { escapeValue: false },
})

function group(over: Partial<UsageGroup>): UsageGroup {
  return {
    key: 'k',
    total: 0,
    compute: 0,
    storage: 0,
    egress: 0,
    idle_share: 0,
    previous: 0,
    delta: 0,
    daily: [1, 2, 3],
    ...over,
  }
}

function report(over: Partial<UsageReport> = {}): UsageReport {
  return {
    period: { from: '2026-10-01T00:00:00Z', to: '2026-10-04T00:00:00Z' },
    previous: { from: '2026-09-01T00:00:00Z', to: '2026-09-04T00:00:00Z' },
    days: ['2026-10-01', '2026-10-02', '2026-10-03'],
    totals: { total: 12, allocated: 8, unallocated: 4, previous: 10 },
    resources: [
      group({
        key: 'app:web',
        kind: 'app',
        name: 'web',
        total: 6,
        previous: 5,
        delta_pct: 20,
      }),
    ],
    projects: [
      group({ key: 'p1', total: 8, previous: 5, delta_pct: 60 }),
      group({ key: 'unassigned', total: 2 }),
    ],
    environments: [],
    teams: [],
    movers: [],
    forecast: 30,
    currency: 'USD',
    mode: 'node',
    basis: 'avg',
    labels: { 'project:p1': 'Storefront' },
    unpriced_nodes: [],
    includes_unallocated: true,
    configured: true,
    telemetry_available: true,
    generated_at: '2026-10-03T12:00:00Z',
    ...over,
  }
}

function renderWith(ui: React.ReactNode) {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('usageFormat', () => {
  it('formats deltas with a sign and a dash when unknown', () => {
    expect(formatDeltaPct(12.4)).toBe('+12%')
    expect(formatDeltaPct(-8)).toBe('-8%')
    expect(formatDeltaPct(undefined)).toBe('-')
  })

  it('draws no sparkline for a single point', () => {
    expect(sparklinePoints([4])).toBe('')
    expect(sparklinePoints([1, 3]).split(' ')).toHaveLength(2)
  })

  it('folds projects beyond the limit into one other series', () => {
    const r = report({
      projects: [
        group({ key: 'a', daily: [1, 1, 1] }),
        group({ key: 'b', daily: [2, 2, 2] }),
        group({ key: 'c', daily: [3, 3, 3] }),
      ],
    })
    const s = stackSeries(r, 1, { other: 'Other', unassigned: 'None' })
    expect(s.map((x) => x.key)).toEqual(['a', '__other'])
    expect(s[1]?.values).toEqual([5, 5, 5])
  })
})

describe('UsageTable', () => {
  it('names projects from labels and switches dimension', async () => {
    renderWith(<UsageTable report={report()} />)
    expect(screen.getByText('Storefront')).toBeInTheDocument()
    expect(screen.getByText('Unassigned')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'App or database' }),
    )
    expect(screen.getByText('web')).toBeInTheDocument()
    expect(screen.getByText('+20%')).toBeInTheDocument()
  })

  it('says so when a dimension has no rows', async () => {
    renderWith(<UsageTable report={report()} />)
    await userEvent.click(screen.getByRole('button', { name: 'Team' }))
    expect(screen.getByText(en.table.empty)).toBeInTheDocument()
  })
})

describe('AppCostChip', () => {
  function stub(body: unknown) {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve(body),
        } as unknown as Response),
      ),
    )
  }

  it('shows the app cost and its share of the forecast', async () => {
    stub(report())
    renderWith(<AppCostChip appName="web" />)
    expect(await screen.findByTestId('app-cost-chip')).toHaveTextContent(
      'Cost this month',
    )
  })

  it('renders nothing before prices are configured', async () => {
    stub(report({ configured: false }))
    renderWith(<AppCostChip appName="web" />)
    await new Promise((r) => setTimeout(r, 20))
    expect(screen.queryByTestId('app-cost-chip')).toBeNull()
  })
})
