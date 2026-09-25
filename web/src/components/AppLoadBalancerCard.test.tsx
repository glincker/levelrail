import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppLoadBalancerCard } from './AppLoadBalancerCard'
import type { Brand } from '../types/brand'

vi.mock('../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: 'https://test.example/support',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: 'https://test.example/docs',
    DiscussionsURL: '',
  }),
}))

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

interface Calls {
  puts: unknown[]
}

function stubFetch(resource: unknown, calls: Calls) {
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      (input, init) => {
        const url = urlOf(input)
        if (init?.method === 'PUT') {
          calls.puts.push(JSON.parse(init.body as string))
          return Promise.resolve(
            jsonResponse({
              app_name: 'demo',
              configured: true,
              config: calls.puts.at(-1),
              algorithms: [],
              export_formats: [],
            }),
          )
        }
        if (url.endsWith('/loadbalancer/status')) {
          return Promise.resolve(
            jsonResponse({
              service: 'demo',
              algorithm: 'least_conn',
              ready: false,
              reason: 'UpstreamsDegraded',
              message: '1 of 2 replicas are in the pool',
              observed_at: '2026-01-01T00:00:00Z',
              upstreams: [
                {
                  id: 'demo#0',
                  dial: '127.0.0.1:30000',
                  replica: 0,
                  weight: 0,
                  state: 'healthy',
                  healthy: true,
                  active_connections: 3,
                  fails: 0,
                },
                {
                  id: 'demo#1',
                  dial: '',
                  replica: 1,
                  weight: 0,
                  state: 'unhealthy',
                  healthy: false,
                  active_connections: 0,
                  fails: 0,
                  reason: 'container not running',
                },
              ],
            }),
          )
        }
        if (url.endsWith('/loadbalancer')) {
          return Promise.resolve(jsonResponse(resource))
        }
        throw new Error(`unexpected fetch: ${url}`)
      },
    ),
  )
}

function renderCard(replicas = 2) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <AppLoadBalancerCard appName="demo" replicas={replicas} />
    </QueryClientProvider>,
  )
}

describe('AppLoadBalancerCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('offers setup when no load balancer is configured and saves the chosen algorithm', async () => {
    const calls: Calls = { puts: [] }
    stubFetch(
      {
        app_name: 'demo',
        configured: false,
        algorithms: [],
        export_formats: [],
      },
      calls,
    )
    const user = userEvent.setup()
    renderCard()

    await user.click(
      await screen.findByRole('button', { name: 'Set up load balancer' }),
    )
    await user.click(screen.getByRole('radio', { name: /Least connections/ }))
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(calls.puts).toHaveLength(1))
    expect(calls.puts[0]).toEqual({ algorithm: 'least_conn' })
  })

  it('shows weight sliders only for the weighted algorithm', async () => {
    const calls: Calls = { puts: [] }
    stubFetch(
      {
        app_name: 'demo',
        configured: true,
        config: { algorithm: 'weighted', weights: [3, 1] },
        algorithms: [],
        export_formats: [],
      },
      calls,
    )
    const user = userEvent.setup()
    renderCard()

    expect(await screen.findByLabelText('Weight for replica 0')).toHaveValue(
      '3',
    )
    await user.click(screen.getByRole('radio', { name: /Round robin/ }))
    expect(screen.queryByLabelText('Weight for replica 0')).toBeNull()
  })

  it('renders the live upstream table for a configured balancer', async () => {
    stubFetch(
      {
        app_name: 'demo',
        configured: true,
        config: { algorithm: 'least_conn' },
        algorithms: [],
        export_formats: [],
      },
      { puts: [] },
    )
    renderCard()

    expect(await screen.findByText('127.0.0.1:30000')).toBeInTheDocument()
    expect(screen.getByText('UpstreamsDegraded')).toBeInTheDocument()
    expect(screen.getByText('container not running')).toBeInTheDocument()
    expect(screen.getByText('1 of 2 healthy')).toBeInTheDocument()
  })

  it('blocks saving while a field is invalid', async () => {
    stubFetch(
      {
        app_name: 'demo',
        configured: true,
        config: {},
        algorithms: [],
        export_formats: [],
      },
      { puts: [] },
    )
    const user = userEvent.setup()
    renderCard()

    await user.type(await screen.findByLabelText('Drain on deploy'), 'soon')
    expect(screen.getByText(/Use a duration such as/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  })
})
