import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { HealthCheckEditor } from './HealthCheckEditor'
import type { AppDetail } from '../types/appDetail'
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
  }),
}))

function fakeApp(overrides: Partial<AppDetail> = {}): AppDetail {
  return {
    name: 'demo-app',
    image: 'demo-app:latest',
    port: 3000,
    strategy: 'rolling',
    replicas: 1,
    bind_address: 'private',
    suspended: false,
    env_dirty: false,
    ...overrides,
  }
}

function renderEditor(app: AppDetail) {
  const fetchMock = vi.fn((_input: RequestInfo | URL, init?: RequestInit) =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve(JSON.parse((init?.body as string | undefined) ?? '{}')),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fetchMock)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <HealthCheckEditor app={app} />
    </QueryClientProvider>,
  )
  return fetchMock
}

function sentBody(fetchMock: ReturnType<typeof vi.fn>): AppDetail {
  const put = fetchMock.mock.calls.find(
    (call) => (call[1] as RequestInit | undefined)?.method === 'PUT',
  )
  expect(put).toBeDefined()
  return JSON.parse((put?.[1] as RequestInit).body as string) as AppDetail
}

describe('HealthCheckEditor', () => {
  it('saves an HTTPS readiness probe with redirect and status settings', async () => {
    const fetchMock = renderEditor(fakeApp())
    const user = userEvent.setup()

    await user.click(screen.getByRole('switch', { name: 'Readiness probe' }))
    await user.type(
      screen.getByLabelText('Path', { selector: '#readiness-path' }),
      '/health',
    )
    await user.click(screen.getByRole('switch', { name: 'HTTPS' }))
    await user.click(
      screen.getByRole('switch', { name: /Skip TLS verification/ }),
    )
    await user.click(screen.getByRole('switch', { name: /Follow redirects/ }))
    await user.type(
      screen.getByLabelText('Expected status', {
        selector: '#readiness-expected-status',
      }),
      '200-399',
    )
    await user.click(screen.getByRole('button', { name: 'Save health checks' }))

    await waitFor(() => {
      expect(sentBody(fetchMock).health?.readiness).toEqual({
        path: '/health',
        scheme: 'https',
        tls_skip_verify: true,
        follow_redirects: false,
        expected_status: '200-399',
      })
    })
  })

  it('saves an exec liveness probe as a shell command', async () => {
    const fetchMock = renderEditor(fakeApp())
    const user = userEvent.setup()

    await user.click(screen.getByRole('switch', { name: 'Liveness probe' }))
    await user.click(
      screen.getByRole('switch', { name: /Run a command instead of HTTP/ }),
    )
    await user.type(screen.getByLabelText('Command'), 'redis-cli ping')
    await user.click(screen.getByRole('button', { name: 'Save health checks' }))

    await waitFor(() => {
      expect(sentBody(fetchMock).health?.liveness).toEqual({
        path: '',
        exec: ['/bin/sh', '-c', 'redis-cli ping'],
      })
    })
  })

  it('rejects a malformed expected status without saving', async () => {
    const fetchMock = renderEditor(fakeApp())
    const user = userEvent.setup()

    await user.click(screen.getByRole('switch', { name: 'Readiness probe' }))
    await user.type(
      screen.getByLabelText('Path', { selector: '#readiness-path' }),
      '/',
    )
    await user.type(
      screen.getByLabelText('Expected status', {
        selector: '#readiness-expected-status',
      }),
      '399-200',
    )
    await user.click(screen.getByRole('button', { name: 'Save health checks' }))

    expect(await screen.findByText(/runs backwards/)).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('round-trips an existing exec probe and keeps ready_timeout', async () => {
    const app = fakeApp({
      health: {
        liveness: {
          path: '',
          exec: ['/bin/sh', '-c', 'pg_isready -U app'],
          failures: 3,
        },
        ready_timeout: 90_000_000_000,
      },
    })
    const fetchMock = renderEditor(app)
    const user = userEvent.setup()

    expect(
      screen.getByText(/Currently: exec pg_isready -U app/),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Command')).toHaveValue('pg_isready -U app')
    await user.click(screen.getByRole('button', { name: 'Save health checks' }))

    await waitFor(() => {
      const body = sentBody(fetchMock)
      expect(body.health?.liveness).toEqual({
        path: '',
        exec: ['/bin/sh', '-c', 'pg_isready -U app'],
        failures: 3,
      })
      expect(body.health?.ready_timeout).toBe(90_000_000_000)
    })
  })
})
