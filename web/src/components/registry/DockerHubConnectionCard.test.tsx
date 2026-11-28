import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ReactNode } from 'react'
import hubEn from '../../locales/en/dockerHubConnection.json'
import { DockerHubConnectionCard } from './DockerHubConnectionCard'
import { DockerHubTagBrowser } from './DockerHubTagBrowser'
import type { Brand } from '../../types/brand'
import type { RegistryCredential } from '../../types/registryCredential'
import type {
  DockerHubLimits,
  DockerHubStatus,
} from '../../types/dockerHubAccount'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('../../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: 'https://test.example/support',
    PrimaryColor: 'black',
    LogoSVG: '',
    DocsURL: 'https://test.example/docs',
    DiscussionsURL: '',
    RepoURL: '',
  }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dockerHubConnection'],
  defaultNS: 'dockerHubConnection',
  resources: { en: { dockerHubConnection: hubEn } },
  interpolation: { escapeValue: false },
})

function withI18n(node: ReactNode) {
  return (
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>{node}</I18nextProvider>
    </QueryClientProvider>
  )
}

const credential: RegistryCredential = {
  id: 'c1',
  name: 'hub',
  registry_host: 'docker.io',
  username: 'me',
  created_at: '2026-01-01T00:00:00Z',
}

const limits: DockerHubLimits = {
  authenticated: true,
  known: true,
  limit: 200,
  remaining: 150,
  window_seconds: 21600,
  source: 'account',
  low: false,
  warn_percent: 20,
  use_for_pulls: true,
  checked_at: '2026-01-01T00:00:00Z',
  cached: false,
}

function status(over: Partial<DockerHubStatus> = {}): DockerHubStatus {
  return {
    valid: true,
    kind: 'personal',
    namespace: 'me',
    scopes: ['repo:read'],
    capabilities: [],
    private_repos_visible: true,
    expiry_known: false,
    expires_at: null,
    permission_note: '',
    limits,
    checked_at: '2026-01-01T00:00:00Z',
    cached: false,
    ...over,
  }
}

function json(body: unknown, statusCode = 200): Promise<Response> {
  return Promise.resolve({
    ok: statusCode < 400,
    status: statusCode,
    json: () => Promise.resolve(body),
  } as unknown as Response)
}

function mockFetch(handler: (url: string) => Promise<Response>) {
  const fn = vi.fn((input: RequestInfo | URL) =>
    handler(
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.href
          : input.url,
    ),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderCard() {
  render(withI18n(<DockerHubConnectionCard credentials={[credential]} />))
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DockerHubConnectionCard', () => {
  it('shows a valid token with its pull limit', async () => {
    mockFetch(() => json(status()))
    renderCard()
    expect(await screen.findByText('Valid')).toBeTruthy()
    expect(screen.getByText(/150 of 200 pulls left/)).toBeTruthy()
    expect(screen.getByText(/Scopes: repo:read/)).toBeTruthy()
  })

  it('shows loading while the status is pending', () => {
    mockFetch(() => new Promise<Response>(() => undefined))
    renderCard()
    expect(screen.getAllByText('Checking the token...').length).toBeGreaterThan(
      0,
    )
  })

  it('offers Re-test and Replace token for a rejected token', async () => {
    mockFetch(() =>
      json(
        status({
          valid: false,
          message: 'Docker Hub rejected the token.',
          limits: null,
        }),
      ),
    )
    renderCard()
    expect(await screen.findByText('Rejected')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Re-test' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Replace token' }))
    expect(screen.getByLabelText('New access token')).toBeTruthy()
  })

  it('warns when the remaining pulls are low', async () => {
    mockFetch(() =>
      json(status({ limits: { ...limits, remaining: 10, low: true } })),
    )
    renderCard()
    expect(
      await screen.findByText(/Low: under 20% of the limit remains/),
    ).toBeTruthy()
  })

  it('shows an error with a Re-test button when the status call fails', async () => {
    mockFetch(() => json({ error: 'could not reach Docker Hub' }, 502))
    renderCard()
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Re-test' })).toBeTruthy()
  })

  it('refreshes the pull limit', async () => {
    const fn = mockFetch((url) =>
      url.includes('/limits')
        ? json({ ...limits, remaining: 100 })
        : json(status()),
    )
    renderCard()
    fireEvent.click(await screen.findByRole('button', { name: 'Refresh' }))
    await waitFor(() =>
      expect(screen.getByText(/100 of 200 pulls left/)).toBeTruthy(),
    )
    expect(fn).toHaveBeenCalledWith(
      '/api/v1/registry-credentials/c1/limits?refresh=true',
    )
  })

  it('renders nothing without a Docker Hub credential', () => {
    const { container } = render(
      withI18n(
        <DockerHubConnectionCard
          credentials={[{ ...credential, registry_host: 'ghcr.io' }]}
        />,
      ),
    )
    expect(container.textContent).toBe('')
  })
})

describe('DockerHubTagBrowser', () => {
  it('filters tags and picks one', async () => {
    mockFetch(() => json({ tags: ['latest', '1.0', '1.1', 'edge'] }))
    const onPick = vi.fn()
    render(
      withI18n(
        <DockerHubTagBrowser
          credentialId="c1"
          repository="me/app"
          onPick={onPick}
        />,
      ),
    )
    fireEvent.change(await screen.findByLabelText('Filter tags'), {
      target: { value: '1.' },
    })
    expect(screen.queryByText('latest')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Use tag 1.1' }))
    expect(onPick).toHaveBeenCalledWith('1.1')
  })
})
