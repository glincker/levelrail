import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainMaintenanceControl } from './DomainMaintenanceControl'
import maintenanceEn from '../locales/en/maintenance.json'
import domainsEn from '../locales/en/domains.json'
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
  ns: ['domains', 'maintenance'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn, maintenance: maintenanceEn } },
  interpolation: { escapeValue: false },
})

const DOMAIN = 'app.example.com'
const ENFORCED = {
  mode: 'levelrail',
  maintenance: true,
  basic_auth: true,
  auth_gate: true,
  waf: true,
  redirect: true,
  error_pages: true,
}

interface Call {
  url: string
  method: string
  body?: string
}

function renderWith(maintenance: Record<string, unknown>) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({
        url,
        method: init?.method ?? 'GET',
        body: typeof init?.body === 'string' ? init.body : undefined,
      })
      const body = url.includes('/maintenance')
        ? init?.method === 'PUT'
          ? { domain: DOMAIN, enabled: true }
          : maintenance
        : [
            {
              domain: DOMAIN,
              service_name: 'web',
              waf_enabled: false,
              has_redirect: false,
              maintenance_enabled: true,
              has_basic_auth: false,
              enforcement: ENFORCED,
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
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <DomainMaintenanceControl appName="web" domain={DOMAIN} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
  return calls
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DomainMaintenanceForm', () => {
  it('sends the page copy, retry and bypass list', async () => {
    const calls = renderWith({ domain: DOMAIN, enabled: false })
    const user = userEvent.setup()
    await user.click(await screen.findByText('Page and schedule'))
    await user.type(screen.getByLabelText('Page title'), 'Upgrading')
    await user.type(screen.getByLabelText('Retry after (minutes)'), '10')
    await user.type(
      screen.getByLabelText('Addresses that still reach the app'),
      '203.0.113.7{enter}10.0.0.0/8',
    )
    await user.click(
      screen.getByRole('switch', {
        name: 'Let uptime probes reach the readiness path',
      }),
    )
    await user.click(screen.getByText('Enable maintenance with these settings'))
    await user.click(
      await screen.findByRole('button', { name: 'Turn maintenance on' }),
    )
    await waitFor(() => {
      expect(calls.some((c) => c.method === 'PUT')).toBe(true)
    })
    const put = calls.find((c) => c.method === 'PUT')
    expect(JSON.parse(put?.body ?? '{}')).toEqual({
      title: 'Upgrading',
      retry_after_seconds: 600,
      bypass: ['203.0.113.7', '10.0.0.0/8'],
      bypass_health_check: false,
    })
  })

  it('summarises the end time and bypass rules while on', async () => {
    renderWith({
      domain: DOMAIN,
      enabled: true,
      until: '2030-01-02T03:04:00Z',
      window_id: 'mw_1',
      bypass: ['203.0.113.7/32'],
      bypass_health_check: true,
    })
    expect(await screen.findByText(/^Ends /)).toBeTruthy()
    expect(screen.getByText('1 address bypasses the page')).toBeTruthy()
    expect(
      screen.getByText(
        'Uptime probes on the readiness path still reach the app',
      ),
    ).toBeTruthy()
  })
})
