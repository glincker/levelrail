import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ProxyProviderCard } from '../ProxyProviderCard'
import { clearStoredUsername, setStoredUsername } from '../../lib/authStore'
import type {
  ProxyIntegration,
  ProxyProvider,
} from '../../queries/proxyIntegration'
import {
  jsonResponse,
  proxyIntegration,
  renderWithProviders,
  stubFetch,
} from '../../test/proxyTestUtils'

const BASE = '/api/v1/system/proxy-integration'
const INSTALL = `POST ${BASE}/install`
const ME = 'ops@example.com'

const PROVIDERS: ProxyProvider[] = [
  { kind: 'embedded', mode: 'builtin', detected: false, selected: false },
  { kind: 'traefik', mode: 'managed', detected: false, selected: true },
  { kind: 'nginx', mode: 'managed', detected: false, selected: false },
]

function missing(): ProxyIntegration {
  const base = proxyIntegration()
  return {
    ...base,
    provider: 'traefik',
    providers: PROVIDERS,
    domains: [],
    detected: {
      kind: 'none',
      container: '',
      image: '',
      published_ports: [],
      dynamic_dir: '',
      entrypoint_http: '',
      entrypoint_https: '',
      cert_resolver: '',
      upstream_host: '',
      complete: false,
      missing: ['no running container publishes port 80 or 443 on this host'],
    },
    settings: { ...base.settings, integration: 'off', dynamic_dir: '' },
    steps: [
      { id: 'detect', state: 'blocked', detail: 'nothing found' },
      { id: 'tls_upstream', state: 'todo', detail: '' },
      { id: 'routes', state: 'todo', detail: 'managed routes are off' },
      { id: 'dns', state: 'todo', detail: 'no domains to check' },
      { id: 'verify', state: 'todo', detail: 'not verified yet' },
    ],
  }
}

const PLAN = {
  provider: 'traefik',
  container: 'acme-traefik',
  image: 'traefik:v3.3.5',
  ports: [80, 443],
  mounts: [
    {
      host: '/data/proxy/traefik/dynamic',
      container: '/etc/traefik/dynamic',
      read_only: false,
    },
  ],
  files: [
    {
      path: '/data/proxy/traefik/traefik.yml',
      content: 'entryPoints:\n  web:\n',
    },
  ],
  restart: 'unless-stopped',
  route_dir: '/data/proxy/traefik/dynamic',
  cert_resolver: '',
  upstream_host: 'host.docker.internal',
}

const root = [
  {
    id: 'u1',
    email: ME,
    display_name: 'Ops',
    has_password: true,
    providers: [],
    abilities: ['root', 'read'],
  },
]

function routes(extra: Record<string, () => Response> = {}) {
  return stubFetch({
    [`GET ${BASE}`]: () => jsonResponse(missing()),
    'GET /api/v1/users': () => jsonResponse(root),
    ...extra,
  })
}

describe('missing proxy state', () => {
  beforeEach(() => setStoredUsername(ME))
  afterEach(() => {
    clearStoredUsername()
    vi.unstubAllGlobals()
  })

  it('shows a calm card and the three choices instead of a red wall', async () => {
    routes()
    renderWithProviders(<ProxyProviderCard />)
    expect(
      await screen.findByText('Traefik is not running on this server yet'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Install and start Traefik for me' }),
    ).toBeEnabled()
    expect(
      screen.getByRole('button', { name: 'Use the built-in Caddy' }),
    ).toBeEnabled()
    expect(
      screen.getByRole('button', { name: 'Enter it by hand' }),
    ).toBeEnabled()
    expect(
      screen.queryByText('Setup cannot continue yet'),
    ).not.toBeInTheDocument()
  })

  it('shows later steps as waiting and explains the disabled apply button', async () => {
    routes()
    renderWithProviders(<ProxyProviderCard />)
    const apply = await screen.findByRole('button', {
      name: 'Apply and verify',
    })
    expect(apply).toBeDisabled()
    expect(apply).toHaveAccessibleDescription(
      /turns on as soon as Traefik is found/,
    )
    expect(screen.getAllByText(/\(Waiting\)/).length).toBe(4)
  })

  it('shows the exact plan first and only installs after an explicit confirm', async () => {
    const user = userEvent.setup()
    const fetchMock = routes({
      [INSTALL]: () => jsonResponse({ dry_run: true, plan: PLAN }),
    })
    renderWithProviders(<ProxyProviderCard />)
    await user.click(
      await screen.findByRole('button', {
        name: 'Install and start Traefik for me',
      }),
    )
    expect(await screen.findByText('traefik:v3.3.5')).toBeInTheDocument()
    expect(screen.getByText(/entryPoints:/)).toBeInTheDocument()
    const bodies = () =>
      fetchMock.mock.calls
        .filter(([u, i]) => u === `${BASE}/install` && i?.method === 'POST')
        .map(([, i]) => JSON.parse(i?.body as string) as { confirm: boolean })
    expect(bodies()).toEqual([{ provider: 'traefik', confirm: false }])
    await user.click(screen.getByRole('button', { name: 'Confirm and start' }))
    await waitFor(() => expect(bodies()).toHaveLength(2))
    expect(bodies()[1]).toMatchObject({ confirm: true })
  })

  it('shows a specific refusal when a port is in use and changes nothing', async () => {
    const user = userEvent.setup()
    routes({
      [INSTALL]: () =>
        jsonResponse(
          {
            code: 'port_in_use',
            error:
              'port 80 is already in use on this server by the process nginx.',
          },
          409,
        ),
    })
    renderWithProviders(<ProxyProviderCard />)
    await user.click(
      await screen.findByRole('button', {
        name: 'Install and start Traefik for me',
      }),
    )
    expect(await screen.findByText('Nothing was changed')).toBeInTheDocument()
    expect(screen.getByText(/port 80 is already in use/)).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Confirm and start' }),
    ).not.toBeInTheDocument()
  })

  it('goes back to the built-in Caddy without touching saved data', async () => {
    const user = userEvent.setup()
    const put = vi.fn(() => jsonResponse({}))
    routes({ 'PUT /api/v1/settings/proxy-integration': put })
    renderWithProviders(<ProxyProviderCard />)
    await user.click(
      await screen.findByRole('button', { name: 'Use the built-in Caddy' }),
    )
    await waitFor(() => expect(put).toHaveBeenCalled())
  })

  it('reveals prefilled manual fields with inline validation and a re-check', async () => {
    const user = userEvent.setup()
    routes()
    renderWithProviders(<ProxyProviderCard />)
    await user.click(
      await screen.findByRole('button', { name: 'Enter it by hand' }),
    )
    const dir = await screen.findByLabelText('Route directory on this server')
    expect(dir).toHaveValue('/etc/traefik/dynamic')
    expect(screen.getByLabelText('HTTP entrypoint')).toHaveValue('web')
    await user.clear(dir)
    await user.type(dir, 'relative/path')
    expect(
      screen.getByText('Enter an absolute path that starts with /.'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Save these values' }),
    ).toBeDisabled()
    expect(
      screen.getAllByRole('button', { name: 'Re-check' }).length,
    ).toBeGreaterThan(0)
  })

  it('offers no automatic install for nginx', async () => {
    const user = userEvent.setup()
    routes()
    renderWithProviders(<ProxyProviderCard />)
    await user.click(await screen.findByRole('radio', { name: /^nginx/ }))
    expect(
      await screen.findByText('nginx is not running on this server yet'),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Install and start/ }),
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(/Automatic install is available for Traefik only/),
    ).toBeInTheDocument()
  })
})
