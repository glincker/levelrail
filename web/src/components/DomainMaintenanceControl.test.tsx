import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainMaintenanceControl } from './DomainMaintenanceControl'
import domainsEn from '../locales/en/domains.json'
import type { DomainEnforcement } from '../lib/domainEnforcement'
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

vi.mock('./HelpLink', () => ({
  HelpLink: ({ label }: { label: string }) => <a href="/help">{label}</a>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

const DOMAIN = 'app.example.com'

function enforcement(
  mode: DomainEnforcement['mode'],
  on: boolean,
  reason?: DomainEnforcement['reason'],
): DomainEnforcement {
  return {
    mode,
    maintenance: on,
    basic_auth: on,
    auth_gate: on,
    waf: on,
    redirect: on,
    error_pages: on,
    reason,
  }
}

function renderControl(e: DomainEnforcement, maintenanceOn = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const body = url.includes('/maintenance')
        ? { domain: DOMAIN, enabled: maintenanceOn }
        : [
            {
              domain: DOMAIN,
              service_name: 'web',
              waf_enabled: false,
              has_redirect: false,
              maintenance_enabled: maintenanceOn,
              has_basic_auth: false,
              enforcement: e,
            },
          ]
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
  const queryClient = createTestQueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={testI18n}>
        <DomainMaintenanceControl appName="web" domain={DOMAIN} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DomainMaintenanceControl enforcement', () => {
  it('never shows maintenance as on when the proxy path is unverified', async () => {
    renderControl(enforcement('proxy:traefik', false, 'route_unverified'))
    expect(await screen.findByText('Saved, not enforced')).toBeTruthy()
    expect(
      await screen.findByText(/Traefik routes app.example.com/),
    ).toBeTruthy()
    expect(screen.queryByText('Maintenance mode on')).toBeNull()
    expect(screen.getByText('Open the proxy page')).toBeTruthy()
  })

  it('names an unmanaged proxy and does not claim enforcement', async () => {
    renderControl(enforcement('proxy:external', false, 'proxy_unmanaged'))
    expect(await screen.findByText(/your proxy terminates TLS/)).toBeTruthy()
    expect(screen.queryByText('Maintenance mode on')).toBeNull()
  })

  it('shows maintenance as on when the built-in ingress serves the domain', async () => {
    renderControl(enforcement('levelrail', true))
    expect(await screen.findByText('Maintenance mode on')).toBeTruthy()
    expect(screen.queryByText('Saved, not enforced')).toBeNull()
  })

  it('shows nothing extra while maintenance is off', async () => {
    renderControl(
      enforcement('proxy:traefik', false, 'route_unverified'),
      false,
    )
    expect(await screen.findByText('Not in maintenance')).toBeTruthy()
    expect(screen.queryByText('Saved, not enforced')).toBeNull()
  })
})
