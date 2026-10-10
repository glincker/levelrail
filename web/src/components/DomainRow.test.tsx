import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainRow } from './DomainRow'
import domainsEn from '../locales/en/domains.json'
import type { Domain } from '../queries/domains'

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
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <DomainRow domain={domain} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('DomainRow', () => {
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

  it('shows an icon for each active flag', () => {
    renderRow(
      makeDomain({
        waf_enabled: true,
        has_redirect: true,
        maintenance_enabled: true,
        has_basic_auth: true,
      }),
    )
    expect(screen.getByTitle('WAF enabled')).toBeInTheDocument()
    expect(screen.getByTitle('Redirect configured')).toBeInTheDocument()
    expect(screen.getByTitle('Maintenance mode on')).toBeInTheDocument()
    expect(screen.getByTitle('Basic auth configured')).toBeInTheDocument()
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
})
