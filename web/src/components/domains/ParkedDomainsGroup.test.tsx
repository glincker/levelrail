import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ParkedDomain } from '../../queries/parkedDomains'
import { ParkedDomainsGroup } from './ParkedDomainsGroup'
import domainsEn from '../../locales/en/domains.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function res(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(''),
    clone() {
      return this
    },
  } as unknown as Response
}

function stubFetch(parked: ParkedDomain[]) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') return Promise.resolve(res({}))
    if (url.includes('/attach')) {
      return Promise.resolve(res({ domain: 'a.com', applied: true }))
    }
    if (url.includes('/domains/parked')) return Promise.resolve(res(parked))
    if (url.includes('/environment-domains')) {
      return Promise.resolve(
        res({
          app: 'web',
          default_domains: [],
          routed_domains: [],
          environments: [],
        }),
      )
    }
    if (url.includes('/static-sites')) return Promise.resolve(res([]))
    return Promise.resolve(res([{ name: 'web' }]))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderGroup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <ParkedDomainsGroup />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ParkedDomainsGroup', () => {
  it('renders nothing when no domain is parked', async () => {
    const fetchMock = stubFetch([])
    const { container } = renderGroup()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the Settings saved badge only when has_settings', async () => {
    stubFetch([
      { domain: 'a.com', kind: 'parked', has_settings: true },
      { domain: 'b.com', kind: 'parked', has_settings: false },
    ])
    renderGroup()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    expect(screen.getAllByText('Settings saved')).toHaveLength(1)
  })

  it('shows the Static site badge for a static site origin', async () => {
    stubFetch([
      {
        domain: 'a.com',
        kind: 'parked',
        parked_from: 'docs',
        parked_from_kind: 'static_site',
      },
      { domain: 'b.com', kind: 'parked', parked_from_kind: 'app' },
    ])
    renderGroup()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    expect(screen.getAllByText('Static site')).toHaveLength(1)
  })

  it('shows DNS and TLS badges from the live fields', async () => {
    stubFetch([
      {
        domain: 'a.com',
        kind: 'parked',
        dns: { status: 'pointing' },
        tls: { state: 'issued', days_left: 30 },
      },
      {
        domain: 'b.com',
        kind: 'parked',
        dns: { status: 'not_pointing' },
        tls: { state: 'pending' },
      },
      {
        domain: 'c.com',
        kind: 'parked',
        dns: { status: 'unknown' },
        tls: { state: 'none' },
      },
    ])
    renderGroup()
    expect(await screen.findByText('DNS pointing')).toBeInTheDocument()
    expect(screen.getByText('DNS not pointing')).toBeInTheDocument()
    expect(screen.getByText('DNS unknown')).toBeInTheDocument()
    expect(screen.getByText('30 days left')).toBeInTheDocument()
    expect(screen.getByText('Certificate pending')).toBeInTheDocument()
    expect(screen.getByText('No certificate')).toBeInTheDocument()
  })

  it('attaches through the attach mutation', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch([{ domain: 'a.com', kind: 'parked' }])
    renderGroup()
    await user.click(await screen.findByRole('button', { name: 'Attach' }))
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'web' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: 'Attach' }))
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([u]) => u.includes('/a.com/attach')),
      ).toBe(true)
    })
  })

  it('deletes through the delete mutation after typing the domain', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch([{ domain: 'a.com', kind: 'parked' }])
    renderGroup()
    await user.click(
      await screen.findByRole('button', { name: 'Delete a.com' }),
    )
    await user.type(screen.getByRole('textbox'), 'a.com')
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([u, init]) =>
            u.includes('/parked/a.com') && init?.method === 'DELETE',
        ),
      ).toBe(true)
    })
  })
})
