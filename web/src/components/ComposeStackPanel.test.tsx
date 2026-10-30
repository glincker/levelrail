import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComposeStackPanel } from './ComposeStackPanel'
import { TemplateInstallForm } from './TemplateInstallForm'
import composeEn from '../locales/en/compose.json'
import type { ComposeStack, TemplateManifest } from '../types/compose'
import { createTestQueryClient } from '@/test/testQueryClient'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => navigate,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['compose'],
  defaultNS: 'compose',
  resources: { en: { compose: composeEn } },
  interpolation: { escapeValue: false },
})

const stack: ComposeStack = {
  app_id: 'stack',
  phase: 'degraded',
  revision: 3,
  services: [
    {
      key: 'db',
      name: 'stack-db',
      image: 'postgres:16',
      replicas: 1,
      ready: 'True',
    },
    {
      key: 'api',
      name: 'stack-api',
      image: 'api:v2',
      replicas: 1,
      ready: 'Unknown',
      reason: 'WaitingForDependencyHealthy',
      depends_on: ['db'],
      depends_on_healthy: ['db'],
    },
  ],
}

function stubFetch(routes: Record<string, { status: number; body: unknown }>) {
  const fn = vi.fn((url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`
    const hit = routes[key] ?? { status: 404, body: { error: 'not found' } }
    return Promise.resolve({
      ok: hit.status < 400,
      status: hit.status,
      json: () => Promise.resolve(hit.body),
      text: () => Promise.resolve(JSON.stringify(hit.body)),
      headers: new Headers({ 'Content-Type': 'application/json' }),
    } as unknown as Response)
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function wrap(ui: React.ReactNode) {
  const queryClient = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  navigate.mockReset()
})

describe('ComposeStackPanel', () => {
  it('shows the phase, each service reason and healthy dependencies', async () => {
    stubFetch({
      'GET /api/v1/apps/stack/compose': { status: 200, body: stack },
    })
    wrap(<ComposeStackPanel appId="stack" />)
    expect(await screen.findByText('Degraded')).toBeInTheDocument()
    expect(screen.getByText('Revision 3')).toBeInTheDocument()
    expect(screen.getByText(/WaitingForDependencyHealthy/)).toBeInTheDocument()
    expect(screen.getByText('db (healthy)')).toBeInTheDocument()
  })

  it('renders nothing for an app that was not deployed from compose', async () => {
    const fetchFn = stubFetch({})
    const { container } = wrap(<ComposeStackPanel appId="plain" />)
    await waitFor(() => expect(fetchFn).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  it('recreates one service', async () => {
    const fetchFn = stubFetch({
      'GET /api/v1/apps/stack/compose': { status: 200, body: stack },
      'POST /api/v1/apps/stack/compose/redeploy': {
        status: 202,
        body: { restarted: ['stack-api'] },
      },
    })
    wrap(<ComposeStackPanel appId="stack" />)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Recreate api' }),
    )
    await waitFor(() =>
      expect(fetchFn).toHaveBeenCalledWith(
        '/api/v1/apps/stack/compose/redeploy',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ service: 'api' }),
        }),
      ),
    )
  })
})

const manifest: TemplateManifest = {
  id: 'wikijs',
  name: 'Wiki.js',
  description: 'A wiki.',
  category: 'Productivity',
  documentation_url: 'https://example.com',
  env: [
    { key: 'DB_PASSWORD', label: 'Database password', generate: 'password' },
    { key: 'ADMIN_EMAIL', label: 'Admin email', required: true },
  ],
  domains: [{ service: 'wiki', port: 3000 }],
}

describe('TemplateInstallForm', () => {
  it('asks only for non-generated settings and blocks a missing required one', async () => {
    const fetchFn = stubFetch({})
    wrap(<TemplateInstallForm manifest={manifest} />)
    expect(screen.queryByLabelText('Database password')).not.toBeInTheDocument()
    expect(screen.getByText('Database password')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Install' }))
    expect(
      await screen.findByText('Admin email is required'),
    ).toBeInTheDocument()
    expect(fetchFn).not.toHaveBeenCalled()
  })

  it('installs with settings and a domain, then opens the stack', async () => {
    const fetchFn = stubFetch({
      'POST /api/v1/service-templates/wikijs/install': {
        status: 200,
        body: { app_id: 'wiki', services: [{ name: 'wiki-db' }], revision: 1 },
      },
    })
    wrap(<TemplateInstallForm manifest={manifest} />)
    await userEvent.type(screen.getByLabelText('App name'), 'wiki')
    await userEvent.type(screen.getByLabelText('Admin email'), 'a@b.co')
    await userEvent.type(
      screen.getByLabelText('Domain for wiki (port 3000)'),
      'wiki.example.com',
    )
    await userEvent.click(screen.getByRole('button', { name: 'Install' }))
    await waitFor(() => expect(navigate).toHaveBeenCalled())
    const sent = fetchFn.mock.calls[0]?.[1]?.body
    const body = JSON.parse(typeof sent === 'string' ? sent : '{}') as Record<
      string,
      unknown
    >
    expect(body).toEqual({
      name: 'wiki',
      env: { ADMIN_EMAIL: 'a@b.co' },
      domains: { wiki: 'wiki.example.com' },
    })
  })
})
