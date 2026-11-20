import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GoLivePanel } from './GoLivePanel'
import {
  AppsBaseDomainCard,
  NoProviderEmptyState,
  ZoneHint,
} from './AppsBaseDomainCard'
import domainsEn from '../locales/en/domains.json'
import type { GoLiveResult } from '../queries/goLive'
import { createTestQueryClient } from '@/test/testQueryClient'

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
  const queryClient = createTestQueryClient()
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
      screen.getByRole('link', {
        name: /Connect Cloudflare, Route53 or Google Cloud DNS/,
      }),
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

  it('offers listed zones as suggestions on a miss', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          res({
            domain: 'app.apps.other.org',
            provider: 'route53',
            configured: true,
            found: false,
            message:
              'no zone contains "app.apps.other.org"; the provider lists example.com',
          }),
        ),
      ),
    )
    const onPick = vi.fn()
    renderWith(<ZoneHint domain="apps.other.org" onPick={onPick} />)
    const pick = await screen.findByRole('button', { name: 'example.com' })
    pick.click()
    expect(onPick).toHaveBeenCalledWith('example.com')
    expect(screen.getByText(/route53:ListHostedZones/)).toBeInTheDocument()
  })
})

describe('base domain wildcard record', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('previews a conflict and only replaces on the explicit button', async () => {
    const wildcardBodies: unknown[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.includes('apps-base-domain/wildcard')) {
          wildcardBodies.push(
            JSON.parse(typeof init?.body === 'string' ? init.body : '{}'),
          )
          return Promise.resolve(
            res({
              dry_run: true,
              base_domain: 'apps.example.com',
              dns: {
                domain: '*.apps.example.com',
                dns: 'preview',
                planned: 'conflict',
                record: { name: '*.apps', type: 'A', value: '203.0.113.5' },
                replaced: [
                  { name: '*.apps', type: 'A', value: '198.51.100.9' },
                ],
                message:
                  'a different record exists; it would not be overwritten',
              },
            }),
          )
        }
        if (url.includes('domain-automation')) {
          return Promise.resolve(res({ dns_provider: 'route53' }))
        }
        if (url.includes('/dns/zone')) {
          return Promise.resolve(
            res({ found: true, zone: 'example.com', provider: 'route53' }),
          )
        }
        return Promise.resolve(res({ apps_base_domain: 'apps.example.com' }))
      }),
    )
    renderWith(<AppsBaseDomainCard />)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview wildcard record' }),
    )
    expect(
      await screen.findByText(/A \*\.apps\.example\.com/),
    ).toBeInTheDocument()
    expect(screen.getByText(/Existing: A \*\.apps/)).toBeInTheDocument()
    expect(
      screen.getByText(/will not be overwritten unless you replace/),
    ).toBeInTheDocument()
    expect(wildcardBodies).toEqual([{ confirm: false, replace: false }])
    fireEvent.click(
      screen.getByRole('button', { name: 'Replace the existing record' }),
    )
    await waitFor(() => {
      expect(wildcardBodies[1]).toEqual({ confirm: true, replace: true })
    })
  })
})
