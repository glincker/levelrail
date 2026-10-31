import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainAutomationCard } from './DomainAutomationCard'
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

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
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

const policy = {
  auto_dns: true,
  auto_proxy_route: false,
  force_https: true,
  www_policy: 'off',
  attach_www_counterpart: false,
  wildcard_for_base_domain: false,
  verify_after: true,
  dns_provider: 'cloudflare',
  proxy_integration_enabled: false,
}

interface Routes {
  automation?: unknown
  runs?: unknown[]
  apps?: unknown[]
}

function bodyText(body: BodyInit | null | undefined): string {
  return typeof body === 'string' ? body : ''
}

function stubApi(routes: Routes = {}) {
  const calls: { url: string; init?: RequestInit }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, init })
      let body: unknown = {}
      if (url.includes('/settings/domain-automation')) {
        body =
          init?.method === 'PUT'
            ? { ...policy, ...(JSON.parse(bodyText(init.body)) as object) }
            : (routes.automation ?? policy)
      } else if (url.includes('/domains/automation/runs')) {
        body = routes.runs ?? []
      } else if (url.startsWith('/api/v1/apps')) {
        body = routes.apps ?? []
      }
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
  return calls
}

function renderCard() {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <DomainAutomationCard />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('DomainAutomationCard', () => {
  it('renders one switch per policy flag reflecting the saved policy', async () => {
    stubApi()
    renderCard()
    const dns = await screen.findByRole('switch', {
      name: 'Create DNS records',
    })
    expect(dns).toBeChecked()
    expect(screen.getByRole('switch', { name: 'Force HTTPS' })).toBeChecked()
    expect(
      screen.getByRole('switch', { name: 'Create proxy routes' }),
    ).not.toBeChecked()
  })

  it('disables DNS automation without a provider and proxy routes without the integration', async () => {
    stubApi({ automation: { ...policy, dns_provider: 'none' } })
    renderCard()
    expect(
      await screen.findByRole('switch', { name: 'Create DNS records' }),
    ).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('switch', { name: 'Create proxy routes' }),
    ).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('switch', { name: 'Force HTTPS' }),
    ).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('saves the whole policy when a switch is flipped', async () => {
    const calls = stubApi()
    renderCard()
    await userEvent.click(
      await screen.findByRole('switch', { name: 'Verify after adding' }),
    )
    await waitFor(() => {
      expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true)
    })
    const put = calls.find((c) => c.init?.method === 'PUT')
    expect(JSON.parse(bodyText(put?.init?.body))).toMatchObject({
      verify_after: false,
      auto_dns: true,
      force_https: true,
    })
  })

  it('saves the www policy from the select', async () => {
    const calls = stubApi()
    renderCard()
    await userEvent.selectOptions(
      await screen.findByLabelText('www policy'),
      'redirect_to_apex',
    )
    await waitFor(() => {
      expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true)
    })
    const put = calls.find((c) => c.init?.method === 'PUT')
    expect(JSON.parse(bodyText(put?.init?.body))).toMatchObject({
      www_policy: 'redirect_to_apex',
    })
  })

  it('asks for an app before previewing a plan', async () => {
    stubApi({ apps: [] })
    renderCard()
    expect(
      await screen.findByText('Create an app first to preview a plan.'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Show plan' })).toBeDisabled()
  })

  it('shows the empty history copy and an undo button for an undoable run', async () => {
    stubApi({
      runs: [
        {
          id: 'run_1',
          app: 'web',
          domain: 'app.example.com',
          result: 'applied',
          steps: [{ id: 'dns', state: 'done' }],
          created_at: '2026-10-18T00:00:00Z',
          undoable: true,
        },
      ],
    })
    renderCard()
    expect(await screen.findByText('app.example.com')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Undo' })).toBeEnabled()
  })

  it('says there are no runs yet', async () => {
    stubApi({ runs: [] })
    renderCard()
    expect(
      await screen.findByText('No automated runs yet.'),
    ).toBeInTheDocument()
  })
})
