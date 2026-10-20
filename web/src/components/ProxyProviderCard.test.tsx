import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ProxyProviderCard } from './ProxyProviderCard'
import { setStoredUsername, clearStoredUsername } from '../lib/authStore'
import type { ProxyProvider } from '../queries/proxyIntegration'
import {
  jsonResponse,
  proxyIntegration,
  renderWithProviders,
  stubFetch,
} from '../test/proxyTestUtils'

const BASE = '/api/v1/system/proxy-integration'
const SETUP = `POST ${BASE}/setup`
const ME = 'ops@example.com'

// Traefik is the detected proxy, so it is the starting choice.
const PROVIDERS: ProxyProvider[] = [
  { kind: 'embedded', mode: 'builtin', detected: false, selected: false },
  { kind: 'traefik', mode: 'managed', detected: true, selected: true },
  { kind: 'nginx', mode: 'managed', detected: false, selected: false },
  { kind: 'caddy', mode: 'manual', detected: false, selected: false },
  { kind: 'haproxy', mode: 'manual', detected: false, selected: false },
]

function fixture() {
  return proxyIntegration({ providers: PROVIDERS, provider: 'traefik' })
}

function usersFor(abilities: string[]) {
  return [
    {
      id: 'u1',
      email: ME,
      display_name: 'Ops',
      has_password: true,
      providers: [],
      abilities,
    },
  ]
}

function setupBodies(fetchMock: ReturnType<typeof stubFetch>) {
  return fetchMock.mock.calls
    .filter(([url, init]) => url === `${BASE}/setup` && init?.method === 'POST')
    .map(
      ([, init]) => JSON.parse(init?.body as string) as Record<string, unknown>,
    )
}

describe('ProxyProviderCard', () => {
  beforeEach(() => {
    setStoredUsername(ME)
  })

  afterEach(() => {
    clearStoredUsername()
    vi.unstubAllGlobals()
  })

  it('renders the detected provider as the selected choice', async () => {
    stubFetch({
      [`GET ${BASE}`]: () => jsonResponse(fixture()),
      'GET /api/v1/users': () => jsonResponse(usersFor(['root', 'read'])),
    })
    renderWithProviders(<ProxyProviderCard />)
    const traefik = await screen.findByRole('radio', { name: /^Traefik/ })
    expect(traefik).toBeChecked()
    expect(screen.getByText('Detected')).toBeInTheDocument()
  })

  it('switches the selected provider when another is chosen', async () => {
    const user = userEvent.setup()
    stubFetch({
      [`GET ${BASE}`]: () => jsonResponse(fixture()),
      'GET /api/v1/users': () => jsonResponse(usersFor(['root', 'read'])),
    })
    renderWithProviders(<ProxyProviderCard />)
    const nginx = await screen.findByRole('radio', { name: /^nginx/ })
    await user.click(nginx)
    expect(nginx).toBeChecked()
    expect(screen.getByRole('radio', { name: /^Traefik/ })).not.toBeChecked()
  })

  it('sends a dry run without confirm when Dry run is pressed', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch({
      [`GET ${BASE}`]: () => jsonResponse(fixture()),
      'GET /api/v1/users': () => jsonResponse(usersFor(['root', 'read'])),
      [SETUP]: () => jsonResponse({ ...fixture(), dry_run: true, changes: [] }),
    })
    renderWithProviders(<ProxyProviderCard />)
    await user.click(await screen.findByRole('button', { name: 'Dry run' }))
    await waitFor(() => expect(setupBodies(fetchMock)).toHaveLength(1))
    expect(setupBodies(fetchMock)[0]).toMatchObject({
      confirm: false,
      provider: 'traefik',
    })
    expect(
      await screen.findByText(
        'Nothing to change. Routes are already in place.',
      ),
    ).toBeInTheDocument()
  })

  it('enables Apply for a root account and sends confirm', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch({
      [`GET ${BASE}`]: () => jsonResponse(fixture()),
      'GET /api/v1/users': () => jsonResponse(usersFor(['root', 'read'])),
      [SETUP]: () => jsonResponse(fixture()),
    })
    renderWithProviders(<ProxyProviderCard />)
    const apply = await screen.findByRole('button', {
      name: 'Apply and verify',
    })
    expect(apply).toBeEnabled()
    await user.click(apply)
    await waitFor(() => expect(setupBodies(fetchMock)).toHaveLength(1))
    expect(setupBodies(fetchMock)[0]).toMatchObject({ confirm: true })
  })

  it('disables Apply and Dry run for a non-root account and explains why', async () => {
    stubFetch({
      [`GET ${BASE}`]: () => jsonResponse(fixture()),
      'GET /api/v1/users': () => jsonResponse(usersFor(['read', 'write'])),
    })
    renderWithProviders(<ProxyProviderCard />)
    expect(
      await screen.findByRole('button', { name: 'Apply and verify' }),
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Dry run' })).toBeDisabled()
    expect(
      screen.getByText('Changing the proxy needs a root account or token.'),
    ).toBeInTheDocument()
  })

  it('shows the error and the next step when the proxy refuses the routes', async () => {
    const user = userEvent.setup()
    stubFetch({
      [`GET ${BASE}`]: () => jsonResponse(fixture()),
      'GET /api/v1/users': () => jsonResponse(usersFor(['root', 'read'])),
      [SETUP]: () =>
        jsonResponse({ error: 'nginx -t failed: unknown directive' }, 409),
    })
    renderWithProviders(<ProxyProviderCard />)
    await user.click(
      await screen.findByRole('button', { name: 'Apply and verify' }),
    )
    expect(
      await screen.findByText('Could not apply the routes.'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('nginx -t failed: unknown directive'),
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'Nothing was kept: the files were rolled back and your proxy keeps serving what it had. Fix the problem below, then try again.',
      ),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Re-check' })).toBeEnabled()
  })
})
