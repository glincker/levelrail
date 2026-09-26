import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AlertHistoryTable } from './AlertHistoryTable'
import { RecentChanges } from './RecentChanges'
import { SloSuggestion } from './SloSuggestion'
import { sloConfigFromForm } from '../queries/sloPreview'
import type { AlertRule } from '../types/alerts'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderWithClient(node: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(<QueryClientProvider client={queryClient}>{node}</QueryClientProvider>)
}

const CHANGES = {
  app: 'web',
  since: '2026-09-26T11:30:00Z',
  until: '2026-09-26T12:00:00Z',
  window_seconds: 1800,
  total: 3,
  changes: [
    {
      at: '2026-09-26T11:58:00Z',
      kind: 'deploy',
      actor: 'bob',
      title: 'Deploy to app@sha256:abcdef012345',
      detail: 'digest abcdef012345',
      likely_cause: true,
    },
    {
      at: '2026-09-26T11:50:00Z',
      kind: 'env',
      actor: 'amy',
      title: 'Env changed: DATABASE_URL',
      keys: ['DATABASE_URL'],
    },
  ],
}

const BURNING = {
  config: { objective: 'availability', target: 99.9 },
  has_traffic: true,
  budget_remaining: 0.42,
  budget_window_requests: 5000,
  windows: [],
  tiers: [
    {
      name: 'page_fast',
      factor: 14.4,
      long_seconds: 3600,
      short_seconds: 300,
      page: true,
      long_burn: 20,
      short_burn: 18,
      effective_burn: 18,
      firing: true,
    },
  ],
  firing: true,
  page: true,
  max_burn: 18,
}

describe('recent changes and SLO UI', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders the change timeline with one likely cause chip and no values', async () => {
    const fetchMock = vi.fn<(input: RequestInfo | URL) => Promise<Response>>(
      () => Promise.resolve(json(CHANGES)),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderWithClient(<RecentChanges app="web" until="2026-09-26T12:00:00Z" />)

    expect(await screen.findByText(/Deploy to app@sha256/)).toBeTruthy()
    expect(screen.getAllByText('Likely cause')).toHaveLength(1)
    expect(screen.getByText(/last 30 minutes/)).toBeTruthy()
    expect(screen.getAllByText(/DATABASE_URL/).length).toBeGreaterThan(0)
    expect(screen.getByText('and 1 more')).toBeTruthy()
    expect(urlOf(fetchMock.mock.calls[0]?.[0] as RequestInfo)).toBe(
      '/api/v1/apps/web/changes?until=2026-09-26T12%3A00%3A00Z',
    )
  })

  it('hides itself when the endpoint is missing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(json({}, 404))),
    )
    renderWithClient(<RecentChanges app="web" />)
    await waitFor(() => {
      expect(screen.queryByTestId('recent-changes')).toBeNull()
    })
  })

  it('expands a fired history row to show what changed before it', async () => {
    const user = userEvent.setup()
    const entry = {
      id: 'ah_1',
      at: '2026-09-26T12:00:00Z',
      rule_id: 'r1',
      rule_name: 'high 5xx',
      rule_kind: 'threshold',
      app: 'web',
      event: 'fired',
      outcome: 'sent',
    }
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) =>
        Promise.resolve(
          json(urlOf(input).includes('/changes') ? CHANGES : [entry]),
        ),
      ),
    )
    renderWithClient(<AlertHistoryTable />)

    await user.click(
      await screen.findByRole('button', {
        name: /what changed before high 5xx/i,
      }),
    )
    expect(await screen.findByText(/Deploy to app@sha256/)).toBeTruthy()
  })

  it('offers a default SLO only for an app with traffic and no slo rule', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(json(BURNING))),
    )
    renderWithClient(<SloSuggestion appName="web" rules={[]} />)
    expect(await screen.findByText(/99.9% availability SLO/)).toBeTruthy()
    expect(
      screen.getByRole('button', { name: /create slo rule/i }),
    ).toBeTruthy()
  })

  it('shows no SLO suggestion when a slo_burn rule exists', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(json(BURNING)))
    vi.stubGlobal('fetch', fetchMock)
    const rule = { id: 'r', name: 's', kind: 'slo_burn' } as AlertRule
    renderWithClient(<SloSuggestion appName="web" rules={[rule]} />)
    await new Promise((r) => setTimeout(r, 30))
    expect(screen.queryByTestId('suggestion')).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('shows no SLO suggestion without traffic', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(json({ ...BURNING, has_traffic: false }))),
    )
    renderWithClient(<SloSuggestion appName="web" rules={[]} />)
    await new Promise((r) => setTimeout(r, 30))
    expect(screen.queryByTestId('suggestion')).toBeNull()
  })

  it('builds the SLO config from form values', () => {
    expect(sloConfigFromForm('availability', '99.9', '300')).toEqual({
      objective: 'availability',
      target: 99.9,
    })
    expect(sloConfigFromForm('latency', 99, '250')).toEqual({
      objective: 'latency',
      target: 99,
      latency_ms: 250,
    })
  })
})
