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
  deletes: URLSearchParams[]
  conflict: boolean
  forbidden: boolean
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
        nameservers: ['a.ns.cf'],
        delegated: 'yes',
        current_nameservers: ['a.ns.cf'],
      },
    ],
    puts: [],
    deletes: [],
    conflict: false,
    forbidden: false,
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
      if (url.includes('/dns/zones/cloudflare/z1'))
        return ok({
          ...state.zones[0],
          registrar_hint: { name: 'Namecheap', url: 'https://reg.example' },
          capabilities: {},
        })
      if (url.includes('/dns/zones/example.com/records')) {
        const u = new URL(url, 'http://x')
        if (init?.method === 'DELETE') {
          state.deletes.push(u.searchParams)
          if (state.conflict && u.searchParams.get('force') !== 'true')
            return Promise.resolve({
              ok: false,
              status: 409,
              json: () =>
                Promise.resolve({
                  error: 'managed',
                  managed_by_levelrail: true,
                }),
            })
          return ok({ deleted: {} })
        }
        return ok({
          zone: state.zones[0],
          provider: 'cloudflare',
          capabilities: {},
          total: 2,
          matched: 2,
          records: [
            {
              name: 'www.example.com',
              type: 'A',
              ttl: 300,
              values: ['1.2.3.4'],
              read_only: false,
              managed_by_levelrail: true,
            },
            {
              name: 'example.com',
              type: 'NS',
              ttl: 300,
              values: ['a.ns.cf'],
              read_only: true,
            },
          ],
        })
      }
      if (state.forbidden && url.endsWith('/api/v1/dns/zones'))
        return Promise.resolve({
          ok: false,
          status: 403,
          json: () => Promise.resolve({ error: 'forbidden' }),
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

  it.each([
    ['yes', 'Delegated'],
    ['no', 'Not delegated'],
    ['partial', 'Partial'],
    ['unknown', 'Unknown'],
  ])('shows the %s delegation chip', async (delegated, label) => {
    mockApi({
      zones: [
        {
          id: 'z1',
          name: 'example.com',
          provider: 'cloudflare',
          delegated,
          delegation_reason:
            delegated === 'unknown' ? 'lookup_failed' : undefined,
          name_servers: [],
        },
      ],
    })
    renderPage()
    expect((await screen.findAllByText(label)).length).toBeGreaterThan(0)
    if (delegated === 'unknown')
      expect(screen.getByTitle(/failed/)).toBeInTheDocument()
  })

  it('lists hosted zones with the record count', async () => {
    mockApi()
    renderPage()
    expect(
      await screen.findByRole('button', { name: 'Open zone example.com' }),
    ).toBeInTheDocument()
    expect(await screen.findByText('Delegated')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
  })

  it('shows a calm state when zones are admin only', async () => {
    mockApi({ forbidden: true })
    renderPage()
    expect(
      await screen.findByText('Zones are managed by instance admins'),
    ).toBeInTheDocument()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('shows lock and Levelrail marks, sends confirm and retries with force on 409', async () => {
    const api = mockApi({ conflict: true })
    renderPage()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Open zone example.com' }),
    )
    expect(await screen.findByText('Levelrail')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /Read only/ })).toBeInTheDocument()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Delete www.example.com A' }),
    )
    await userEvent.type(
      screen.getByLabelText('Type www.example.com to confirm'),
      'www.example.com',
    )
    await userEvent.click(screen.getByRole('button', { name: 'Delete record' }))
    await userEvent.click(
      await screen.findByRole('button', { name: 'Delete anyway' }),
    )
    await waitFor(() => expect(api.deletes).toHaveLength(2))
    expect(api.deletes[0]?.get('confirm')).toBe('www.example.com')
    expect(api.deletes[0]?.get('force')).toBeNull()
    expect(api.deletes[1]?.get('force')).toBe('true')
  })

  it('shows the registrar hint and current name servers', async () => {
    mockApi()
    renderPage()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Open zone example.com' }),
    )
    await userEvent.click(screen.getByRole('tab', { name: 'Name servers' }))
    expect(
      await screen.findByRole('link', { name: 'Open Namecheap' }),
    ).toHaveAttribute('href', 'https://reg.example')
    expect(
      screen.getByText('Name servers found in public DNS'),
    ).toBeInTheDocument()
  })
})
