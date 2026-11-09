import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppScrapedMetricsCard } from './AppScrapedMetricsCard'
import { createTestQueryClient } from '@/test/testQueryClient'
import { AllI18n } from '@/test/allI18n'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function json(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function stubFetch(names: string[]) {
  const fetchMock = vi.fn<
    (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
  >((input) => {
    const url = urlOf(input)
    if (url.endsWith('/scrape-target')) {
      return Promise.resolve(
        json({ configured: true, port: 9100, path: '/metrics' }),
      )
    }
    if (url.includes('/scraped-metrics?')) {
      return Promise.resolve(
        json({
          metric: 'evil',
          series: [
            {
              labels: '{a="<img src=x onerror=alert(1)>"}',
              points: [
                { timestamp: '2026-11-01T00:00:00Z', value: 1 },
                { timestamp: '2026-11-01T00:01:00Z', value: 2 },
              ],
            },
          ],
        }),
      )
    }
    if (url.endsWith('/scraped-metrics')) {
      return Promise.resolve(json({ names }))
    }
    throw new Error(`unexpected fetch: ${url}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderCard(node: ReactNode) {
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <AllI18n>{node}</AllI18n>
    </QueryClientProvider>,
  )
}

describe('AppScrapedMetricsCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows the empty state when nothing is stored', async () => {
    stubFetch([])
    renderCard(<AppScrapedMetricsCard appName="web" />)
    expect(await screen.findByText(/No series stored yet/)).toBeTruthy()
  })

  it('renders scraped labels as text, never as markup', async () => {
    stubFetch(['evil'])
    const user = userEvent.setup()
    renderCard(<AppScrapedMetricsCard appName="web" />)
    await screen.findByRole('option', { name: 'evil' })
    await user.selectOptions(screen.getByLabelText('Metric'), 'evil')
    expect(
      await screen.findByText('evil{a="<img src=x onerror=alert(1)>"}'),
    ).toBeTruthy()
    expect(document.querySelector('img')).toBeNull()
  })

  it('rejects an invalid port and path before saving', async () => {
    const fetchMock = stubFetch([])
    const user = userEvent.setup()
    renderCard(<AppScrapedMetricsCard appName="web" />)
    // Wait for the loaded target so the form has remounted with its values.
    const port = await screen.findByDisplayValue('9100')
    await user.clear(port)
    await user.type(port, '70000')
    await user.click(screen.getByRole('button', { name: 'Save target' }))
    expect(
      await screen.findByText('Enter a port from 1 to 65535.'),
    ).toBeTruthy()
    await user.clear(port)
    await user.type(port, '9100')
    const path = screen.getByLabelText('Path')
    await user.clear(path)
    await user.type(path, '//evil.example/m')
    await user.click(screen.getByRole('button', { name: 'Save target' }))
    expect(
      await screen.findByText('Use a plain path like /metrics.'),
    ).toBeTruthy()
    expect(
      fetchMock.mock.calls.some(([, init]) => init?.method === 'PUT'),
    ).toBe(false)
  })
})
