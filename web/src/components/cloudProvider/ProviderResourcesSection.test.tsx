import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ReactNode } from 'react'
import cloudEn from '../../locales/en/cloudProvider.json'
import { CLOUD_PROVIDERS } from '../../lib/cloudProviderBrand'
import { ProviderResourcesSection } from './ProviderResourcesSection'
import { useProviderLabels } from './useProviderLabels'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => vi.fn(),
}))
vi.mock('../nodes/DestroyServerAction', () => ({
  DestroyServerAction: ({ nodeId }: { nodeId: string }) => (
    <button type="button">destroy {nodeId}</button>
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
    <ProviderResourcesSection
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

const row = (over: Record<string, unknown>) => ({
  id: '1',
  name: 'web-1',
  region: 'nyc3',
  size: 's-1vcpu-1gb',
  status: 'active',
  public_ipv4: '1.2.3.4',
  created_at: new Date().toISOString(),
  tags: [],
  estimated_monthly_price: 6,
  currency: 'USD',
  managed_by_levelrail: false,
  enrolled_node: null,
  ...over,
})

function list(resources: unknown[]) {
  return {
    provider: 'digitalocean',
    account_id: 'digitalocean-default',
    generated_at: new Date().toISOString(),
    cached: false,
    count: resources.length,
    resources,
  }
}

const json = (body: unknown, init: ResponseInit = { status: 200 }) =>
  new Response(JSON.stringify(body), init)

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('ProviderResourcesSection', () => {
  it('separates managed from discovered, shows totals and filters', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        json(
          list([
            row({
              managed_by_levelrail: true,
              enrolled_node: { id: 'n1', name: 'web-1' },
            }),
            row({ id: '2', name: 'old-box', estimated_monthly_price: 12 }),
          ]),
        ),
      ),
    )
    renderSection()
    expect(await screen.findByText('Managed by Levelrail')).toBeTruthy()
    expect(screen.getByText('Discovered', { selector: 'span' })).toBeTruthy()
    expect(screen.getByText('destroy n1')).toBeTruthy()
    expect(screen.getAllByRole('button', { name: /destroy/ })).toHaveLength(1)
    expect(screen.getByText('$18.00')).toBeTruthy()
    fireEvent.click(screen.getByLabelText('Managed'))
    expect(screen.queryByText('old-box')).toBeNull()
    expect(screen.getAllByText('$6.00')).toHaveLength(2)
  })

  it('shows the scope missing state naming the scope', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        json(
          {
            error: 'scope missing',
            code: 'provider_scope_missing',
            needs: 'droplet:read',
          },
          { status: 424 },
        ),
      ),
    )
    renderSection()
    expect(await screen.findByText(/droplet:read/)).toBeTruthy()
    expect(screen.getByText('Open permissions panel')).toBeTruthy()
  })

  it('shows an empty state', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json(list([]))))
    renderSection()
    expect(
      await screen.findByText('No Droplets found in this account.'),
    ).toBeTruthy()
  })

  it('shows the polite message on a 429 refresh', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(json(list([row({})])))
      .mockResolvedValueOnce(
        json(
          { error: 'slow down' },
          { status: 429, headers: { 'Retry-After': '9' } },
        ),
      )
    vi.stubGlobal('fetch', fetchMock)
    renderSection()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(
      await screen.findByText('Just refreshed. Try again in 9 seconds.'),
    ).toBeTruthy()
  })
})
