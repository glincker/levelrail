import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { LoadBalancerOverview } from './LoadBalancerOverview'
import type { Brand } from '../types/brand'
import type { LoadBalancerSummary } from '../queries/loadBalancers'

const navigate = vi.fn()

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigate,
    Link: ({
      children,
      to,
      params,
      ...rest
    }: {
      children?: ReactNode
      to?: string
      params?: Record<string, string>
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to?.replace('$name', params?.name ?? '')} {...rest}>
        {children}
      </a>
    ),
  }
})

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

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as Response
}

function summary(over: Partial<LoadBalancerSummary>): LoadBalancerSummary {
  return {
    app: 'web',
    service: 'web',
    algorithm: 'round_robin',
    state: 'balancing',
    upstreams_total: 2,
    upstreams_healthy: 2,
    config_updated_at: '2026-09-25T00:00:00Z',
    active_health_check: false,
    ...over,
  }
}

function stubFetch(
  items: LoadBalancerSummary[],
  apps: { name: string }[] = [],
) {
  const urls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.toString()
            : input.url
      urls.push(url)
      if (url.startsWith('/api/v1/loadbalancers')) {
        return Promise.resolve(
          jsonResponse({ items, total: items.length, limit: 500, offset: 0 }),
        )
      }
      return Promise.resolve(jsonResponse(apps))
    }),
  )
  return urls
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <LoadBalancerOverview />
    </QueryClientProvider>,
  )
}

describe('LoadBalancerOverview', () => {
  // jsdom has no layout, so give the virtualized scroll container a size.
  beforeAll(() => {
    Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
      configurable: true,
      value: 600,
    })
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
      configurable: true,
      value: 800,
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    navigate.mockReset()
  })

  it('explains the feature and picks an app from the empty state', async () => {
    stubFetch([], [{ name: 'api' }, { name: 'web' }])
    renderPage()

    expect(await screen.findByText('No load balancers yet')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Configure a load balancer' }),
    )
    await userEvent.type(await screen.findByLabelText('Search apps'), 'we')
    expect(screen.queryByRole('button', { name: 'api' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'web' }))

    expect(navigate).toHaveBeenCalledWith({
      to: '/apps/$name/loadbalancer',
      params: { name: 'web' },
    })
  })

  it('points at app creation when there are no apps', async () => {
    stubFetch([], [])
    renderPage()

    await userEvent.click(
      await screen.findByRole('button', { name: 'Configure a load balancer' }),
    )
    expect(await screen.findByText(/You have no apps yet/)).toBeTruthy()
    expect(
      screen.getByText('Create an app').closest('a')?.getAttribute('href'),
    ).toBe('/apps')
  })

  it('lists balancers with state and upstream health, linking to the app tab', async () => {
    stubFetch([
      summary({ app: 'web' }),
      summary({
        app: 'api',
        state: 'degraded',
        upstreams_healthy: 1,
        algorithm: 'least_conn',
      }),
    ])
    renderPage()

    const link = await screen.findByRole('link', { name: 'web' })
    expect(link.getAttribute('href')).toBe('/apps/web/loadbalancer')
    expect(screen.getByText('2 of 2 healthy')).toBeTruthy()
    expect(screen.getByText('1 of 2 healthy')).toBeTruthy()
    expect(screen.getByText('Degraded', { selector: 'span' })).toBeTruthy()
  })

  it('sends the state and search filters to the API', async () => {
    const urls = stubFetch([summary({ app: 'web' })])
    renderPage()
    await screen.findByRole('link', { name: 'web' })

    await userEvent.click(screen.getByRole('button', { name: 'Degraded' }))
    await waitFor(() => {
      expect(urls.some((u) => u.includes('state=degraded'))).toBe(true)
    })
    await userEvent.type(screen.getByLabelText('Search load balancers'), 'we')
    await waitFor(() => {
      expect(urls.some((u) => u.includes('q=we'))).toBe(true)
    })
  })
})
