import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/dnsProviders.json'
import { DnsProvidersPage } from './DnsProvidersPage'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { dnsProviders: en } },
  defaultNS: 'dnsProviders',
  interpolation: { escapeValue: false },
})

interface Mock {
  cf: object
  gcp: object
  zones: object[]
  puts: unknown[]
  gcpTest: object
}

function mockApi(m: Partial<Mock> = {}) {
  const state: Mock = {
    cf: { enabled: true, has_token: true },
    gcp: { enabled: true, has_key: true, client_email: 'sa@x.iam' },
    zones: [
      {
        id: 'z1',
        name: 'example.com',
        provider: 'cloudflare',
        status: 'active',
        record_count: 4,
        name_servers: ['a.ns.cf'],
      },
    ],
    puts: [],
    gcpTest: { ok: false, error: 'permission denied' },
    ...m,
  }
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const ok = (b: unknown) =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve(b),
        })
      if (url.includes('/settings/cloudflare-dns')) {
        if (init?.method === 'PUT') {
          state.puts.push(JSON.parse(init.body as string))
          return ok({ enabled: true, has_token: true })
        }
        return ok(state.cf)
      }
      if (url.includes('/settings/route53-dns'))
        return ok({
          enabled: false,
          has_access_key_id: false,
          has_secret_access_key: false,
        })
      if (url.includes('/settings/google-cloud-dns/test'))
        return ok(state.gcpTest)
      if (url.includes('/settings/google-cloud-dns')) return ok(state.gcp)
      if (url.includes('/dns/zones') && url.includes('/delegation'))
        return ok({
          delegation: {
            domain: 'example.com',
            state: 'delegated',
            expected: [],
            resolvers: [],
          },
          checked_at: '',
        })
      if (url.includes('/dns/zones'))
        return ok({
          provider: 'cloudflare',
          providers: [],
          capabilities: {},
          zones: state.zones,
        })
      if (url.includes('/domains/dns-provider')) {
        const d = new URL(url, 'http://x').searchParams.get('domain')
        return ok({
          domain: d,
          apex: d,
          zone: d,
          nameservers: [],
          provider: d === 'other.org' ? 'route53' : 'cloudflare',
          domain_connect: { supported: false },
          connected: d !== 'other.org',
          connectable: true,
          can_issue_certificates: false,
          hint: '',
        })
      }
      if (url.endsWith('/api/v1/domains')) return ok([{ domain: 'other.org' }])
      return ok({})
    }),
  )
  return state
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <DnsProvidersPage />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => window.history.replaceState(null, '', '/'))
afterEach(() => vi.unstubAllGlobals())

describe('DnsProvidersPage', () => {
  it('shows one tile per provider with its status', async () => {
    mockApi()
    renderPage()
    const tiles = within(
      await screen.findByRole('list', { name: 'DNS providers' }),
    )
    expect(await tiles.findAllByText('Connected')).toHaveLength(2)
    expect(tiles.getByText('Not connected')).toBeInTheDocument()
    expect(screen.getByText('More providers soon')).toBeInTheDocument()
  })

  it('opens the provider sheet and shows the permission line, no form before', async () => {
    mockApi()
    renderPage()
    expect(screen.queryByLabelText('Cloudflare API token')).toBeNull()
    await userEvent.click(
      await screen.findByRole('button', { name: /Open Cloudflare/ }),
    )
    expect(
      await screen.findByLabelText('Cloudflare API token'),
    ).toHaveAttribute('type', 'password')
    expect(screen.getByText(/Zone:Read and DNS:Edit/)).toBeInTheDocument()
  })

  it('renders per-step test results and flags the tile', async () => {
    mockApi()
    renderPage()
    await userEvent.click(
      await screen.findByRole('button', { name: /Open Google Cloud DNS/ }),
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Test connection' }),
    )
    const list = await screen.findByRole('list', { name: 'Test results' })
    expect(list).toHaveTextContent('Credential stored')
    expect(list).toHaveTextContent('permission denied')
  })

  it('opens the matching sheet from the hash anchor', async () => {
    mockApi()
    window.history.replaceState(null, '', '/#route53')
    renderPage()
    expect(
      await screen.findByLabelText('AWS secret access key'),
    ).toBeInTheDocument()
  })

  it('clears the token field after saving and never echoes it', async () => {
    const api = mockApi({ cf: { enabled: false, has_token: false } })
    renderPage()
    await userEvent.click(
      await screen.findByRole('button', { name: /Open Cloudflare/ }),
    )
    const input = await screen.findByLabelText('Cloudflare API token')
    await userEvent.type(input, 'sekret-token')
    await userEvent.click(
      screen.getByRole('button', { name: 'Save and connect' }),
    )
    await waitFor(() => expect(api.puts).toHaveLength(1))
    await waitFor(() => expect(input).toHaveValue(''))
    expect(document.body.textContent).not.toContain('sekret-token')
  })

  it('lists domains with detection and offers to connect a detected host', async () => {
    mockApi()
    renderPage()
    expect(await screen.findByText('other.org')).toBeInTheDocument()
    const connect = await screen.findByRole('button', {
      name: 'Connect Amazon Route 53',
    })
    await userEvent.click(connect)
    expect(
      await screen.findByLabelText('AWS access key ID'),
    ).toBeInTheDocument()
  })

  it('lists hosted zones', async () => {
    mockApi()
    renderPage()
    expect(
      await screen.findByRole('button', { name: 'Open zone example.com' }),
    ).toBeInTheDocument()
    expect(await screen.findByText('Delegated')).toBeInTheDocument()
  })
})
