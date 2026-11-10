import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AccessLogSearchPanel from './AccessLogSearchPanel'
import observabilityEn from '../../locales/en/observability.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['observability'],
  defaultNS: 'observability',
  resources: { en: { observability: observabilityEn } },
  interpolation: { escapeValue: false },
})

function page(records: unknown[]) {
  return new Response(
    JSON.stringify({
      records,
      truncated: false,
      from: '2026-11-09T10:00:00Z',
      to: '2026-11-09T11:00:00Z',
      max_rows: 200,
      max_window_seconds: 86400,
    }),
    { status: 200, headers: { 'Content-Type': 'application/json' } },
  )
}

function renderPanel() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <AccessLogSearchPanel appName="web" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AccessLogSearchPanel', () => {
  it('renders rows as plain text', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          page([
            {
              time: '2026-11-09T10:30:00Z',
              host: 'a.example.com',
              method: 'GET',
              path: '/<b>x</b>',
              status: 502,
              duration_ms: 12,
              bytes_in: 0,
              bytes_out: 0,
              request_id: 'r1',
            },
          ]),
        ),
      ),
    )
    renderPanel()
    expect(
      await screen.findByText('a.example.com/<b>x</b>'),
    ).toBeInTheDocument()
    expect(screen.getByText('502')).toBeInTheDocument()
  })

  it('shows the empty state and sends the chosen filters', async () => {
    const fetchMock = vi.fn<(url: string) => Promise<Response>>(() =>
      Promise.resolve(page([])),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderPanel()
    expect(
      await screen.findByText(observabilityEn.accessSearch.empty),
    ).toBeInTheDocument()
    await userEvent.selectOptions(
      screen.getByLabelText(observabilityEn.accessSearch.status),
      '5xx',
    )
    await userEvent.type(
      screen.getByLabelText(observabilityEn.accessSearch.path),
      '/api/',
    )
    await userEvent.click(
      screen.getByRole('button', { name: observabilityEn.accessSearch.search }),
    )
    await waitFor(() => {
      const url = fetchMock.mock.calls.at(-1)?.[0] ?? ''
      expect(url).toContain('/api/v1/apps/web/access/search?')
      expect(url).toContain('status=5xx')
      expect(url).toContain('path=%2Fapi%2F')
    })
  })
})
