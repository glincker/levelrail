import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ReactNode } from 'react'
import cloudEn from '../../locales/en/cloudProvider.json'
import { CLOUD_PROVIDERS } from '../../lib/cloudProviderBrand'
import { ProviderSpendSection } from './ProviderSpendSection'
import { useProviderLabels } from './useProviderLabels'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

const STUB_CONNECTION = {
  openConnect: () => undefined,
  askDisconnect: () => undefined,
  dialogs: null,
}

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['cloudProvider'],
  defaultNS: 'cloudProvider',
  resources: { en: { cloudProvider: cloudEn } },
  interpolation: { escapeValue: false },
})

function Section() {
  const labels = useProviderLabels('digitalocean')
  return (
    <ProviderSpendSection
      provider={CLOUD_PROVIDERS.digitalocean}
      resource={{ provider: 'digitalocean', has_token: true }}
      labels={labels}
      connection={STUB_CONNECTION}
    />
  )
}

function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <Section />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const now = new Date().toISOString()
const val = (value: string, source: string) => ({
  value,
  source,
  data_as_of: now,
})

function report(over: Record<string, unknown> = {}) {
  return {
    provider: 'digitalocean',
    account_id: 'digitalocean-default',
    currency: 'USD',
    generated_at: now,
    cached: false,
    balance: {
      state: 'ok',
      account_balance: val('-12.50', 'digitalocean_balance'),
      month_to_date_balance: null,
      month_to_date_usage: val('3.25', 'digitalocean_balance'),
      generated_at: now,
    },
    last_invoice: {
      state: 'ok',
      amount: val('40.00', 'digitalocean_billing_history'),
      date: now,
    },
    estimate: {
      state: 'ok',
      monthly_total: val('18.00', 'list_prices'),
      resource_count: 3,
      priced_count: 2,
    },
    ...over,
  }
}

const json = (body: unknown, init: ResponseInit = { status: 200 }) =>
  new Response(JSON.stringify(body), init)

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('ProviderSpendSection', () => {
  it('shows each value with its source and the lag note', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json(report())))
    renderSection()
    expect(await screen.findByText('$40.00')).toBeTruthy()
    expect(screen.getByText('$3.25')).toBeTruthy()
    expect(screen.getByText('$18.00')).toBeTruthy()
    expect(screen.getByText(/from list prices/)).toBeTruthy()
    expect(screen.getByText(/can lag/)).toBeTruthy()
  })

  it('degrades one section to a scope message and keeps the rest', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        json(
          report({
            last_invoice: {
              state: 'scope_missing',
              needs: 'billing:read',
              amount: null,
              date: null,
            },
          }),
        ),
      ),
    )
    renderSection()
    expect(await screen.findByText(/Needs billing:read/)).toBeTruthy()
    expect(screen.getByText('$18.00')).toBeTruthy()
  })

  it('shows the polite message on a 429 refresh', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce(json(report()))
        .mockResolvedValueOnce(
          json(
            { error: 'slow' },
            { status: 429, headers: { 'Retry-After': '30' } },
          ),
        ),
    )
    renderSection()
    await screen.findByText('$40.00')
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(
      await screen.findByText('Just refreshed. Try again in 30 seconds.'),
    ).toBeTruthy()
  })
})
