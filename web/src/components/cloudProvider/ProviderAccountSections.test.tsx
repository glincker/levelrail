import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ReactNode } from 'react'
import cloudEn from '../../locales/en/cloudProvider.json'
import accountsEn from '../../locales/en/cloudAccounts.json'
import { CLOUD_PROVIDERS } from '../../lib/cloudProviderBrand'
import { ProviderCapabilitiesPanel } from './ProviderCapabilitiesPanel'
import { ProviderResourcesSection } from './ProviderResourcesSection'
import { ProviderSpendSection } from './ProviderSpendSection'
import { useProviderLabels } from './useProviderLabels'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => vi.fn(),
}))
vi.mock('../../hooks/useCloudAccountActions', () => ({
  useCloudAccountActions: () => ({
    view: true,
    viewSpend: true,
    operate: false,
    verify: true,
    probe: true,
    remove: false,
  }),
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
  ns: ['cloudProvider', 'cloudAccounts'],
  defaultNS: 'cloudProvider',
  resources: { en: { cloudProvider: cloudEn, cloudAccounts: accountsEn } },
  interpolation: { escapeValue: false },
})

const RESOURCE = {
  provider: 'oracle',
  has_token: false,
  connected: true,
  accounts: [
    {
      id: 'oracle-test',
      label: 'oracle-test',
      status: 'active',
      auth_method: 'api_key',
      needs_relogin: false,
      managed: false,
    },
  ],
}

function json(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function stubApi() {
  const fetchMock = vi.fn((input: string) => {
    const url = input
    if (url.endsWith('/instances')) {
      return json([
        {
          id: 'ocid1.instance.a',
          name: 'vm-a',
          region: 'us-ashburn-1',
          state: 'running',
          raw_state: 'RUNNING',
          shape: 'VM.Standard.A1.Flex',
          created_at: '2026-11-01T00:00:00Z',
          tags: {},
          managed: false,
          read_only: false,
        },
      ])
    }
    if (url.endsWith('/spend')) {
      return json({
        account_id: 'oracle-test',
        provider: 'oracle',
        period_start: '2026-11-01',
        period_end: '2026-11-30',
        state: 'ok',
        totals: [{ currency: 'USD', amount: 12.5 }],
        by_service: [],
        source: 'usage_api',
        estimated: false,
        fetched_at: '2026-11-21T00:00:00Z',
        stale: false,
      })
    }
    if (url.endsWith('/probe')) {
      return json({
        capabilities: [
          { name: 'compute', ok: true, verified: true, method: 'api' },
          { name: 'usage', ok: true, verified: true, method: 'api' },
          {
            name: 'budgets',
            ok: false,
            verified: true,
            method: 'api',
            missing: ['read budgets'],
          },
        ],
      })
    }
    return Promise.resolve(new Response('{}', { status: 404 }))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function Sections() {
  const labels = useProviderLabels('oracle')
  const props = {
    provider: CLOUD_PROVIDERS.oracle,
    resource: RESOURCE,
    labels,
    connection: STUB_CONNECTION,
  }
  return (
    <>
      <ProviderResourcesSection {...props} />
      <ProviderSpendSection {...props} />
      <ProviderCapabilitiesPanel {...props} />
    </>
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('account-backed provider sections', () => {
  it('renders Oracle instances, spend and the probe from the cloud account', async () => {
    const fetchMock = stubApi()
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <I18nextProvider i18n={testI18n}>
          <Sections />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    expect(await screen.findByText('vm-a')).toBeTruthy()
    expect(screen.getByText('VM.Standard.A1.Flex')).toBeTruthy()
    expect(await screen.findByText('$12.50')).toBeTruthy()
    await waitFor(() => {
      expect(screen.getByText('Missing access')).toBeTruthy()
    })
    expect(screen.getByText('read budgets')).toBeTruthy()
    const urls = fetchMock.mock.calls.map((c) => c[0])
    expect(
      urls.some((u) => u.includes('/cloud-accounts/oracle-test/instances')),
    ).toBe(true)
    expect(urls.some((u) => u.includes('/node-providers/oracle/'))).toBe(false)
  })
})
