import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import cloudEn from '../../locales/en/cloudProvider.json'
import { CLOUD_PROVIDERS } from '../../lib/cloudProviderBrand'
import { ProviderCapabilitiesPanel } from './ProviderCapabilitiesPanel'
import { useProviderLabels } from './useProviderLabels'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['cloudProvider'],
  defaultNS: 'cloudProvider',
  resources: { en: { cloudProvider: cloudEn } },
  interpolation: { escapeValue: false },
})

function Panel({ connected }: { connected: boolean }) {
  const labels = useProviderLabels('digitalocean')
  return (
    <ProviderCapabilitiesPanel
      provider={CLOUD_PROVIDERS.digitalocean}
      resource={{ provider: 'digitalocean', has_token: connected }}
      labels={labels}
    />
  )
}

function renderPanel(connected = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <Panel connected={connected} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const cap = (id: string, state: string, needs = '') => ({
  id,
  group: 'compute',
  state,
  evidence: 'probe_ok',
  needs,
  used_by: ['provision'],
  observed_at: new Date().toISOString(),
})

function result(capabilities: unknown[], method = 'probe') {
  return {
    provider: 'digitalocean',
    account_id: 'digitalocean-default',
    method,
    checked_at: new Date().toISOString(),
    cached: false,
    capabilities,
  }
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('ProviderCapabilitiesPanel', () => {
  it('shows granted, missing with the exact scope, and unknown', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify(
              result([
                cap('regions.read', 'granted'),
                cap('billing.balance', 'missing', 'billing:read'),
                cap('instances.create', 'unknown', 'droplet:create'),
              ]),
            ),
          ),
        ),
    )
    renderPanel()
    expect(await screen.findByText('Granted')).toBeTruthy()
    expect(screen.getByText('Missing')).toBeTruthy()
    expect(screen.getByText('Needs billing:read')).toBeTruthy()
    expect(screen.getByText('Unknown')).toBeTruthy()
    expect(screen.getByText('1 of 3 granted')).toBeTruthy()
  })

  it('explains the not connected state without calling the API', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    renderPanel(false)
    expect(screen.getByText('Not connected')).toBeTruthy()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('shows the no probe state when the API returns nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(new Response(JSON.stringify(result([], 'none')))),
    )
    renderPanel()
    expect(
      await screen.findByText('No permission check for DigitalOcean yet'),
    ).toBeTruthy()
  })

  it('shows the wait politely when a re-check is rate limited', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(result([cap('regions.read', 'granted')]))),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: 'slow down' }), {
          status: 429,
          headers: { 'Retry-After': '42' },
        }),
      )
    vi.stubGlobal('fetch', fetchMock)
    renderPanel()
    fireEvent.click(await screen.findByRole('button', { name: 'Re-check' }))
    expect(
      await screen.findByText(
        'Permissions were just checked. Try again in 42 seconds.',
      ),
    ).toBeTruthy()
    expect(fetchMock.mock.calls[1]?.[0]).toContain('?refresh=1')
  })
})
