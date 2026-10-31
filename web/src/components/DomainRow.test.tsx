import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainsTable } from './DomainsTable'
import domainsEn from '../locales/en/domains.json'
import httpsEn from '../locales/en/https.json'
import type { Domain } from '../queries/domains'
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
  ns: ['domains', 'https'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn, https: httpsEn } },
  interpolation: { escapeValue: false },
})

function makeDomain(over: Partial<Domain> = {}): Domain {
  return {
    domain: 'app.example.com',
    service_name: 'web',
    waf_enabled: false,
    has_redirect: false,
    maintenance_enabled: false,
    has_basic_auth: false,
    ...over,
  }
}

function renderRow(domain: Domain) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise<Response>(() => undefined)),
  )
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <DomainsTable
          domains={[domain]}
          certByDomain={new Map()}
          appCount={1}
          onAdd={() => undefined}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('DomainsTable rows', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows no status flags when nothing is configured', () => {
    renderRow(makeDomain())
    expect(screen.queryByTitle('WAF enabled')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Redirect configured')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Maintenance mode on')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Basic auth configured')).not.toBeInTheDocument()
  })

  it('shows the ingress issue instead of a silent fallback', () => {
    renderRow(
      makeDomain({
        issues: [
          {
            domain: 'app.example.com',
            code: 'custom_cert_unresolved',
            severity: 'degraded',
            message: 'key unreadable',
          },
        ],
      }),
    )
    expect(
      screen.getByText(/Uploaded certificate not in use/),
    ).toBeInTheDocument()
  })

  it('shows an icon for each active flag', () => {
    renderRow(
      makeDomain({
        waf_enabled: true,
        has_redirect: true,
        maintenance_enabled: true,
        has_basic_auth: true,
        enforcement: {
          mode: 'levelrail',
          maintenance: true,
          basic_auth: true,
          auth_gate: true,
          waf: true,
          redirect: true,
          error_pages: true,
        },
      }),
    )
    expect(screen.getByTitle('WAF enabled')).toBeInTheDocument()
    expect(screen.getByTitle('Redirect configured')).toBeInTheDocument()
    expect(screen.getByTitle('Maintenance mode on')).toBeInTheDocument()
    expect(screen.getByTitle('Basic auth configured')).toBeInTheDocument()
  })

  it('marks a saved flag the proxy path does not enforce', () => {
    renderRow(
      makeDomain({
        maintenance_enabled: true,
        enforcement: {
          mode: 'proxy:traefik',
          maintenance: false,
          basic_auth: false,
          auth_gate: false,
          waf: false,
          redirect: false,
          error_pages: false,
          reason: 'route_unverified',
        },
      }),
    )
    expect(screen.queryByTitle('Maintenance mode on')).not.toBeInTheDocument()
    expect(
      screen.getByTitle('Maintenance mode on (saved, not enforced)'),
    ).toBeInTheDocument()
  })

  it('shows an unlinked standalone domain with its target and scoped calls', () => {
    renderRow(
      makeDomain({
        domain: 'tools.example.com',
        service_name: '',
        kind: 'standalone',
        target: 'http://127.0.0.1:3000',
      }),
    )
    expect(screen.getByText('Unlinked')).toBeInTheDocument()
    expect(screen.getByText('http://127.0.0.1:3000')).toBeInTheDocument()
    const calls = vi.mocked(fetch).mock.calls.map((c) => c[0] as string)
    expect(
      calls.some((u) =>
        u.includes('/apps/_standalone/domains/tools.example.com/'),
      ),
    ).toBe(true)
    expect(calls.some((u) => u.includes('/apps//domains/'))).toBe(false)
  })

  it('shows the inline certificate failure next to the domain', () => {
    renderRow(
      makeDomain({
        acme_failure: {
          error: 'connection refused',
          renewal: false,
          at: '2026-10-10T00:00:00Z',
          reason: 'unreachable',
          action: 'open_port_80',
        },
      }),
    )
    expect(screen.getByText(/open ports 80 and 443/i)).toBeInTheDocument()
  })

  describe('when a proxy owns TLS', () => {
    const staleFailure = {
      error: 'tls: unrecognized name',
      renewal: false,
      at: '2026-10-10T00:00:00Z',
      reason: 'other_proxy',
      action: 'other_proxy' as const,
    }

    it('hides the stale ACME failure and shows the proxy certificate', () => {
      renderRow(
        makeDomain({
          tls_owner: 'proxy',
          acme_failure: staleFailure,
          proxy_certificate: {
            issuer: 'R11',
            not_after: '2027-01-05T00:00:00Z',
            status: 'healthy',
          },
        }),
      )
      expect(screen.queryByText(/answers port 80/i)).not.toBeInTheDocument()
      expect(screen.queryByText('Request failed')).not.toBeInTheDocument()
      expect(screen.getByText('Proxy certificate')).toBeInTheDocument()
      expect(screen.getByText(/via your proxy/i)).toBeInTheDocument()
    })

    it('says the proxy manages it when the certificate is not known yet', () => {
      renderRow(makeDomain({ tls_owner: 'proxy', acme_failure: staleFailure }))
      expect(screen.getByText('Managed by your proxy')).toBeInTheDocument()
      expect(screen.queryByText(/answers port 80/i)).not.toBeInTheDocument()
    })
  })
})
