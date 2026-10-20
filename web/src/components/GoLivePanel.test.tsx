import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GoLivePanel } from './GoLivePanel'
import { NoProviderEmptyState, ZoneHint } from './AppsBaseDomainCard'
import domainsEn from '../locales/en/domains.json'
import type { GoLiveResult } from '../queries/goLive'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function res(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderWith(ui: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
    </I18nextProvider>,
  )
}

function result(over: Partial<GoLiveResult>): GoLiveResult {
  return {
    app: 'web',
    domain: 'app.example.com',
    state: 'pending',
    steps: [],
    policy: {
      auto_dns: true,
      auto_proxy_route: false,
      force_https: true,
      www_policy: 'off',
      attach_www_counterpart: false,
      wildcard_for_base_domain: false,
      verify_after: true,
    },
    checked_at: '2026-10-10T00:00:00Z',
    ...over,
  }
}

describe('GoLivePanel', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the created record, propagation wait and a Cloudflare badge', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          res(
            result({
              steps: [
                {
                  id: 'dns',
                  state: 'done',
                  provider: 'cloudflare',
                  record: {
                    name: 'app',
                    type: 'A',
                    value: '203.0.113.5',
                    ttl_seconds: 1,
                  },
                },
                {
                  id: 'propagation',
                  state: 'pending',
                  resolvers: [{ name: '1.1.1.1', addresses: ['203.0.113.5'] }],
                },
                { id: 'certificate', state: 'pending' },
              ],
            }),
          ),
        ),
      ),
    )
    renderWith(<GoLivePanel app="web" domain="app.example.com" />)
    await waitFor(() => {
      expect(
        screen.getByText('DNS record created at cloudflare'),
      ).toBeInTheDocument()
    })
    expect(screen.getByText('Created at Cloudflare')).toBeInTheDocument()
    expect(
      screen.getByText('Waiting for DNS to propagate (1.1.1.1 sees it)'),
    ).toBeInTheDocument()
    expect(screen.queryByText('Open live site')).not.toBeInTheDocument()
  })

  it('shows the certificate issuer and a live link when live', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          res(
            result({
              state: 'live',
              url: 'https://app.example.com',
              steps: [
                {
                  id: 'certificate',
                  state: 'done',
                  issuer: "Let's Encrypt",
                  not_after: '2027-01-02T00:00:00Z',
                },
              ],
            }),
          ),
        ),
      ),
    )
    renderWith(<GoLivePanel app="web" domain="app.example.com" />)
    await waitFor(() => {
      expect(
        screen.getByText(/Certificate issued by Let's Encrypt/),
      ).toBeInTheDocument()
    })
    expect(
      screen.getByRole('link', { name: 'Open live site' }),
    ).toHaveAttribute('href', 'https://app.example.com')
  })

  it('prompts to connect Cloudflare only when no provider is configured', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          res(result({ steps: [{ id: 'dns', state: 'manual' }] })),
        ),
      ),
    )
    renderWith(<GoLivePanel app="web" domain="app.example.com" />)
    await waitFor(() => {
      expect(
        screen.getByText(/Connect Cloudflare to do this automatically/),
      ).toBeInTheDocument()
    })
  })

  it('explains a conflict and leaves the record alone', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('offline'))),
    )
    renderWith(
      <GoLivePanel
        app="web"
        domain="app.example.com"
        initial={result({
          state: 'failed',
          steps: [{ id: 'dns', state: 'conflict', provider: 'cloudflare' }],
        })}
        dns={{ domain: 'app.example.com', dns: 'conflict' }}
      />,
    )
    expect(screen.getByText(/was left alone/)).toBeInTheDocument()
  })
})

describe('base domain empty state and zone hint', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the connect-a-provider empty state', () => {
    renderWith(<NoProviderEmptyState />)
    expect(
      screen.getByText(/No DNS provider is connected yet/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Connect Cloudflare or Route53/ }),
    ).toBeInTheDocument()
  })

  it('reports an unmanaged zone and a valid hostname error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          res({
            domain: 'app.apps.other.org',
            provider: 'cloudflare',
            configured: true,
            found: false,
          }),
        ),
      ),
    )
    const { rerender } = renderWith(<ZoneHint domain="apps.other.org" />)
    await waitFor(() => {
      expect(screen.getByText(/No zone for this name/)).toBeInTheDocument()
    })
    rerender(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={new QueryClient()}>
          <ZoneHint domain="not a host" />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    expect(screen.getByText(/Enter a hostname/)).toBeInTheDocument()
  })
})
