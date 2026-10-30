import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import platformEn from '../../locales/en/platformSettings.json'
import {
  PLATFORM_ANCHORS,
  PLATFORM_SETTINGS,
  PLATFORM_PAGE_PATH,
  filterPlatformSettings,
} from '../../lib/platformSettingsIndex'
import { PROVIDER_HASH } from './dns-providers/providers'
import { PlatformOverviewPage } from './PlatformOverviewPage'
import { PlatformStatusStrip } from './PlatformStatusStrip'
import { PlatformAddressesPage } from './PlatformAddressesPage'
import { PlatformHttpsPage } from './PlatformHttpsPage'
import { PlatformIngressPage } from './PlatformIngressPage'
import { DnsProvidersPage } from './DnsProvidersPage'

const navigate = vi.fn()

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigate,
    useRouterState: () => '',
    Link: ({
      children,
      to,
      hash,
      ...rest
    }: {
      children?: ReactNode
      to?: string
      hash?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={hash ? `${to}#${hash}` : to} {...rest}>
        {children}
      </a>
    ),
  }
})

// Cards keep their own tests; here only the page wiring and anchors matter.
vi.mock('../EnableHttpsCard', () => ({ EnableHttpsCard: () => <div /> }))
vi.mock('../settings/CertificatesCard', () => ({
  CertificatesCard: () => <div />,
}))
vi.mock('../IngressSettingsCard', () => ({
  IngressSettingsCard: () => <div />,
}))
vi.mock('../IngressEdgeCard', () => ({ IngressEdgeCard: () => <div /> }))
vi.mock('../IngressConnectivityCard', () => ({
  IngressConnectivityCard: () => <div />,
}))
vi.mock('../ReverseProxyGuideCard', () => ({
  ReverseProxyGuideCard: () => <div />,
}))
vi.mock('../DashboardUrlCard', () => ({
  DASHBOARD_URL_ANCHOR: 'dashboard-url',
  DashboardUrlCard: () => <div />,
}))
vi.mock('../AppsBaseDomainCard', () => ({ AppsBaseDomainCard: () => <div /> }))
vi.mock('../FallbackDomainsCard', () => ({
  FallbackDomainsCard: () => <div />,
}))
vi.mock('../DomainAutomationCard', () => ({
  DomainAutomationCard: () => <div />,
}))
vi.mock('../CloudflareDnsCard', () => ({ CloudflareDnsCard: () => <div /> }))
vi.mock('../Route53DnsCard', () => ({ Route53DnsCard: () => <div /> }))
vi.mock('../GoogleCloudDnsCard', () => ({ GoogleCloudDnsCard: () => <div /> }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['platformSettings'],
  defaultNS: 'platformSettings',
  resources: { en: { platformSettings: platformEn } },
  interpolation: { escapeValue: false },
})

interface Api {
  primary: string
  dashboard: string
  cloudflare: boolean
  guidance: string
}

function stubApi(api: Api) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string) => {
      const routes: Record<string, unknown> = {
        '/api/v1/settings/ingress': {
          primary_domain: api.primary,
          acme_enabled: true,
          hsts_enabled: false,
        },
        '/api/v1/settings/dashboard-url': { dashboard_url: api.dashboard },
        '/api/v1/settings/cloudflare-dns': {
          enabled: api.cloudflare,
          has_token: api.cloudflare,
        },
        '/api/v1/settings/route53-dns': {
          enabled: false,
          has_access_key_id: false,
          has_secret_access_key: false,
        },
        '/api/v1/settings/google-cloud-dns': { enabled: false, has_key: false },
        '/api/v1/ingress/connectivity': {
          host: '203.0.113.7',
          guidance: api.guidance,
          ports: [],
        },
      }
      const key = Object.keys(routes)
        .filter((k) => String(input).startsWith(k))
        .sort((x, y) => y.length - x.length)[0]
      const body = key ? routes[key] : []
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
}

function renderWith(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>{node}</QueryClientProvider>
    </I18nextProvider>,
  )
}

beforeEach(() => {
  navigate.mockReset()
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }))
  Object.defineProperty(Element.prototype, 'scrollIntoView', {
    value: vi.fn(),
    configurable: true,
  })
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('platform overview', () => {
  it('warns and offers Enable HTTPS when HTTPS is off', async () => {
    stubApi({ primary: '', dashboard: '', cloudflare: false, guidance: 'ok' })
    renderWith(<PlatformOverviewPage />)
    expect(await screen.findByText('HTTPS not enabled')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Enable HTTPS' })).toHaveAttribute(
      'href',
      '/settings/platform/https',
    )
    expect(
      await screen.findByText('No DNS provider connected'),
    ).toBeInTheDocument()
    expect(await screen.findByText('Ingress healthy')).toBeInTheDocument()
  })

  it('shows the healthy state when HTTPS is on', async () => {
    stubApi({
      primary: 'lr.example.com',
      dashboard: 'https://lr.example.com',
      cloudflare: true,
      guidance: 'ok',
    })
    renderWith(<PlatformOverviewPage />)
    expect(await screen.findByText('HTTPS enabled')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Enable HTTPS' })).toBeNull()
    expect(
      await screen.findByText('https://lr.example.com'),
    ).toBeInTheDocument()
    expect(
      await screen.findByText('DNS: Cloudflare connected'),
    ).toBeInTheDocument()
  })

  it('filters settings by keyword and opens the first hit on Enter', async () => {
    stubApi({ primary: '', dashboard: '', cloudflare: false, guidance: 'ok' })
    const user = userEvent.setup()
    renderWith(<PlatformOverviewPage />)
    const input = screen.getByRole('searchbox', { name: 'Search settings' })
    await user.type(input, 'route53')
    const list = screen.getByRole('list', { name: 'Platform settings results' })
    expect(list.querySelectorAll('li')).toHaveLength(1)
    await user.keyboard('{Enter}')
    expect(navigate).toHaveBeenCalledWith({
      to: '/settings/platform/dns-providers',
      hash: PLATFORM_ANCHORS.route53Dns,
    })
    await user.clear(input)
    await user.type(input, 'zzzz')
    expect(screen.getByText(/No platform setting matches/)).toBeInTheDocument()
  })

  it('focuses the search with /', async () => {
    stubApi({ primary: '', dashboard: '', cloudflare: false, guidance: 'ok' })
    const user = userEvent.setup()
    renderWith(<PlatformOverviewPage />)
    await user.keyboard('/')
    expect(screen.getByRole('searchbox')).toHaveFocus()
  })
})

describe('platform index', () => {
  it('matches every entry by title, keyword or page', () => {
    const text = (id: string) => ({ title: id, description: '' })
    expect(filterPlatformSettings('', text)).toHaveLength(
      PLATFORM_SETTINGS.length,
    )
    expect(filterPlatformSettings('sslip', text).map((e) => e.id)).toContain(
      'fallbackDomains',
    )
    expect(filterPlatformSettings('hosted zone', text)).toHaveLength(1)
  })
})

describe('card anchors', () => {
  const pages = [
    ['https', <PlatformHttpsPage key="h" />],
    ['addresses', <PlatformAddressesPage key="a" />],
    ['dns-providers', <DnsProvidersPage key="d" />],
  ] as const

  it.each(pages)('renders an anchor per card on %s', async (page, node) => {
    stubApi({
      primary: 'x.example.com',
      dashboard: '',
      cloudflare: false,
      guidance: 'ok',
    })
    const { container } = renderWith(node)
    for (const entry of PLATFORM_SETTINGS.filter((e) => e.page === page)) {
      await waitFor(() => {
        expect(container.querySelector(`#${entry.anchor}`)).not.toBeNull()
      })
    }
  })

  it('renders an anchor per card on ingress', async () => {
    stubApi({
      primary: 'x.example.com',
      dashboard: '',
      cloudflare: false,
      guidance: 'ok',
    })
    const { container } = renderWith(<PlatformIngressPage />)
    for (const entry of PLATFORM_SETTINGS.filter((e) => e.page === 'ingress')) {
      await waitFor(() => {
        expect(container.querySelector(`#${entry.anchor}`)).not.toBeNull()
      })
    }
  })

  it('links every entry to a platform page path', () => {
    for (const e of PLATFORM_SETTINGS) {
      expect(PLATFORM_PAGE_PATH[e.page]).toMatch(/^\/settings\/platform\//)
    }
  })
})

describe('dns provider anchors', () => {
  it('use the ids the providers page opens its sheets on', () => {
    expect(PLATFORM_ANCHORS.cloudflareDns).toBe(PROVIDER_HASH.cloudflare)
    expect(PLATFORM_ANCHORS.route53Dns).toBe(PROVIDER_HASH.route53)
    expect(PLATFORM_ANCHORS.googleCloudDns).toBe(PROVIDER_HASH.googleclouddns)
  })
})

describe('domains status strip', () => {
  it('shows the warning and links into the platform pages', async () => {
    stubApi({ primary: '', dashboard: '', cloudflare: false, guidance: 'ok' })
    renderWith(<PlatformStatusStrip />)
    expect(await screen.findByText('HTTPS not enabled')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Platform settings' }),
    ).toHaveAttribute('href', '/settings/platform')
    expect(await screen.findByText('Not set')).toBeInTheDocument()
  })

  it('shows the dashboard address when set', async () => {
    stubApi({
      primary: 'lr.example.com',
      dashboard: 'https://lr.example.com',
      cloudflare: false,
      guidance: 'ok',
    })
    renderWith(<PlatformStatusStrip />)
    expect(await screen.findByText('HTTPS enabled')).toBeInTheDocument()
    expect(
      await screen.findByRole('link', { name: 'https://lr.example.com' }),
    ).toHaveAttribute('href', '/settings/platform/addresses#dashboard-url')
  })
})
