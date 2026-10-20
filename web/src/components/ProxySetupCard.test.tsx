import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProxySetupCard } from './ProxySetupCard'
import {
  jsonResponse,
  proxyDomain,
  proxyIntegration,
  renderWithProviders,
  stubFetch,
} from '../test/proxyTestUtils'

vi.mock('../hooks/useBrand', () => ({
  useBrand: () => ({ DocsURL: 'https://docs.example.com' }),
}))

const BASE = '/api/v1/system/proxy-integration'

describe('ProxySetupCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the five steps with their state and detail text', async () => {
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse(proxyIntegration()) })
    renderWithProviders(<ProxySetupCard />)
    const steps = await screen.findByRole('list', {
      name: 'Make this domain live',
    })
    const items = within(steps).getAllByRole('listitem')
    expect(items).toHaveLength(5)
    expect(items[0]).toHaveTextContent('Find your proxy')
    expect(items[0]).toHaveTextContent('(Done)')
    expect(items[0]).toHaveTextContent('Traefik found')
    expect(items[2]).toHaveTextContent('(To do)')
    expect(items[3]).toHaveTextContent('(Blocked)')
    expect(items[4]).toHaveTextContent('(Error)')
  })

  it('labels detected facts and shows what is missing', async () => {
    const data = proxyIntegration()
    data.detected.complete = false
    data.detected.cert_resolver = ''
    data.detected.missing = ['certificatesResolvers.letsencrypt']
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse(data) })
    renderWithProviders(<ProxySetupCard />)
    expect(await screen.findByText('traefik:v3.1')).toBeInTheDocument()
    expect(screen.getAllByText('detected').length).toBeGreaterThan(0)
    expect(screen.getByText('Setup cannot continue yet')).toBeInTheDocument()
    expect(
      screen.getByText('certificatesResolvers.letsencrypt'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Set up routing' }),
    ).toBeDisabled()
  })

  it('confirms the directory and domains before calling setup', async () => {
    const user = userEvent.setup()
    const data = proxyIntegration({
      steps: [{ id: 'detect', state: 'done', detail: '' }],
    })
    const fetchMock = stubFetch({
      [`GET ${BASE}`]: () => jsonResponse(data),
      [`POST ${BASE}/setup`]: () => jsonResponse(data),
    })
    renderWithProviders(<ProxySetupCard />)
    await user.click(
      await screen.findByRole('button', { name: 'Set up routing' }),
    )
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('/etc/traefik/dynamic')
    expect(dialog).toHaveTextContent('app.example.com')
    expect(
      fetchMock.mock.calls.some(([, init]) => init?.method === 'POST'),
    ).toBe(false)

    await user.click(
      within(dialog).getByRole('button', { name: 'Write routes' }),
    )
    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'POST',
      )
      expect(post?.[0]).toBe(`${BASE}/setup`)
      expect(JSON.parse(post?.[1]?.body as string)).toEqual({
        confirm: true,
        dynamic_dir: '/etc/traefik/dynamic',
      })
    })
  })

  it('shows per-domain state, certificate line and last error', async () => {
    const data = proxyIntegration({
      domains: [
        proxyDomain({ domain: 'live.example.com' }),
        proxyDomain({
          domain: 'wait.example.com',
          certificate: null,
          reachable: true,
        }),
        proxyDomain({
          domain: 'bad.example.com',
          state: 'error',
          certificate: null,
          last_error: 'The proxy returned 502',
        }),
      ],
    })
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse(data) })
    renderWithProviders(<ProxySetupCard />)
    expect(await screen.findByText('Live')).toBeInTheDocument()
    expect(screen.getByText('Waiting for certificate')).toBeInTheDocument()
    expect(screen.getByText('Error')).toBeInTheDocument()
    expect(screen.getByText(/Let's Encrypt, valid until/)).toBeInTheDocument()
    expect(
      screen.getAllByText('Proxy default certificate, waiting for issuance'),
    ).toHaveLength(2)
    expect(
      screen.getByText('Last problem: The proxy returned 502'),
    ).toBeInTheDocument()
  })

  it('verifies one domain with a result toast request', async () => {
    const user = userEvent.setup()
    const data = proxyIntegration({
      domains: [proxyDomain({ certificate: null })],
    })
    const fetchMock = stubFetch({
      [`GET ${BASE}`]: () => jsonResponse(data),
      [`POST ${BASE}/verify`]: () => jsonResponse(proxyDomain()),
    })
    renderWithProviders(<ProxySetupCard />)
    await user.click(await screen.findByRole('button', { name: 'Verify' }))
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([url]) => url === `${BASE}/verify?domain=app.example.com`,
        ),
      ).toBe(true)
    })
  })

  it('stays quiet on 404 and 501', async () => {
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse({}, 501) })
    renderWithProviders(<ProxySetupCard />)
    expect(
      await screen.findByText(
        'Proxy integration is not available on this server version.',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('points to the manual guide when no proxy is detected', async () => {
    const data = proxyIntegration()
    data.detected.kind = 'none'
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse(data) })
    renderWithProviders(<ProxySetupCard />)
    expect(
      await screen.findByText('No existing proxy detected'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Open the manual guide' }),
    ).toBeInTheDocument()
  })

  it('renders nothing once every domain is live', async () => {
    const fetchMock = stubFetch({
      [`GET ${BASE}`]: () =>
        jsonResponse(proxyIntegration({ domains: [proxyDomain()] })),
    })
    const { container } = renderWithProviders(<ProxySetupCard />)
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    await waitFor(() => expect(container).toBeEmptyDOMElement())
  })
})
