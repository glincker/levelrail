import { cleanup, render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ReactNode } from 'react'
import cloudEn from '../../locales/en/cloudProvider.json'
import { CLOUD_PROVIDER_IDS } from '../../lib/cloudProviderBrand'
import { CloudProviderPage } from './CloudProviderPage'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => vi.fn(),
  useSearch: () => ({}),
}))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({
    title,
    actions,
    children,
  }: {
    title: ReactNode
    actions?: ReactNode
    children: ReactNode
  }) => (
    <div>
      <h1>{title}</h1>
      {actions}
      {children}
    </div>
  ),
}))
vi.mock('../nodes/DestroyServerAction', () => ({
  DestroyServerAction: ({ nodeId }: { nodeId: string }) => (
    <button type="button">destroy {nodeId}</button>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['cloudProvider'],
  defaultNS: 'cloudProvider',
  resources: { en: { cloudProvider: cloudEn } },
  interpolation: { escapeValue: false },
})

function respond(provisions: unknown[] = []) {
  return vi.fn((url: string) => {
    const body = url.endsWith('/node-provisions')
      ? provisions
      : url.endsWith('/node-providers')
        ? CLOUD_PROVIDER_IDS.map((p) => ({ provider: p, has_token: true }))
        : url.includes('/regions')
          ? [{ id: 'a', name: 'A' }]
          : {
              provider: 'x',
              account_id: 'x',
              method: 'none',
              checked_at: new Date().toISOString(),
              cached: false,
              capabilities: [],
            }
    return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))
  })
}

function renderPage(id: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <CloudProviderPage providerId={id} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('CloudProviderPage', () => {
  it.each(CLOUD_PROVIDER_IDS)(
    '%s shows its own title, vocabulary and create label',
    async (id) => {
      vi.stubGlobal('fetch', respond())
      renderPage(id)
      const p = cloudEn.providers[id]
      expect(
        screen.getByRole('heading', { level: 1, name: p.name }),
      ).toBeTruthy()
      const chips = screen.getByRole('list', { name: p.name })
      for (const term of p.terms.vocab.split(', ')) {
        expect(within(chips).getByText(term)).toBeTruthy()
      }
      expect(
        await screen.findByRole('button', { name: p.createLabel }),
      ).toBeTruthy()
      expect(
        screen.getByRole('heading', { level: 2, name: p.terms.servers }),
      ).toBeTruthy()
    },
  )

  it('lists provisions for this provider with an empty state otherwise', async () => {
    vi.stubGlobal(
      'fetch',
      respond([
        {
          id: '1',
          provider: 'hetzner',
          region: 'fsn1',
          size: 'cx22',
          name: 'web-1',
          role: 'general',
          status: 'ready',
          ip_address: '203.0.113.9',
          node_id: 'n1',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        },
      ]),
    )
    renderPage('hetzner')
    expect(await screen.findByText('203.0.113.9')).toBeTruthy()
    expect(screen.getByText('destroy n1')).toBeTruthy()
    cleanup()
    renderPage('digitalocean')
    expect(await screen.findByText(/No Droplets created/)).toBeTruthy()
  })

  it('shows a not found state for an unknown provider', () => {
    renderPage('nope')
    expect(screen.getByText('Unknown provider')).toBeTruthy()
  })
})
