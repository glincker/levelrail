import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { HealthCheckAutoDetect } from './HealthCheckAutoDetect'
import type { HealthDiscoveryResult } from '../queries/healthDiscovery'

function renderPanel(
  response: HealthDiscoveryResult | { error: string },
  status = 200,
  onUsePath: (path: string) => void = vi.fn(),
) {
  const fetchMock = vi.fn(() =>
    Promise.resolve({
      ok: status < 400,
      status,
      json: () => Promise.resolve(response),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fetchMock)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <HealthCheckAutoDetect appName="demo-app" onUsePath={onUsePath} />
    </QueryClientProvider>,
  )
  return { fetchMock, onUsePath }
}

describe('HealthCheckAutoDetect', () => {
  it('offers the single clear match with a one-click Use it button', async () => {
    const { fetchMock, onUsePath } = renderPanel({
      name: 'demo-app',
      found: '/healthz',
      attempts: [
        { path: '/healthz', success: true, latency_ms: 12 },
        {
          path: '/health',
          success: false,
          error: 'returned 404',
          latency_ms: 8,
        },
      ],
    })
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Auto-detect' }))

    expect(
      await screen.findByText(/Found a working health check/),
    ).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/apps/demo-app/health/discover',
      expect.objectContaining({ method: 'POST' }),
    )

    await user.click(screen.getByRole('button', { name: 'Use it' }))
    expect(onUsePath).toHaveBeenCalledWith('/healthz')
  })

  it('lists every attempt when nothing matched, never auto-filling a guess', async () => {
    renderPanel({
      name: 'demo-app',
      attempts: [
        {
          path: '/healthz',
          success: false,
          error: 'connection refused',
          latency_ms: 5,
        },
        {
          path: '/health',
          success: false,
          error: 'returned 404',
          latency_ms: 7,
        },
      ],
    })
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Auto-detect' }))

    await waitFor(() => {
      expect(screen.getByText('/healthz')).toBeInTheDocument()
      expect(screen.getByText('connection refused')).toBeInTheDocument()
      expect(screen.getByText('/health')).toBeInTheDocument()
      expect(screen.getByText('returned 404')).toBeInTheDocument()
    })
    expect(
      screen.queryByText(/Found a working health check/),
    ).not.toBeInTheDocument()
  })

  it('offers a Use button on every success when more than one path answered', async () => {
    const { onUsePath } = renderPanel({
      name: 'demo-app',
      attempts: [
        { path: '/healthz', success: true, latency_ms: 10 },
        { path: '/', success: true, latency_ms: 15 },
      ],
    })
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Auto-detect' }))

    const useButtons = await screen.findAllByRole('button', { name: 'Use' })
    expect(useButtons).toHaveLength(2)
    const [firstUseButton] = useButtons
    expect(firstUseButton).toBeDefined()
    await user.click(firstUseButton as HTMLElement)
    expect(onUsePath).toHaveBeenCalledWith('/healthz')
  })

  it('shows the server error message when discovery fails', async () => {
    renderPanel({ error: 'app has no running container' }, 409)
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Auto-detect' }))

    expect(
      await screen.findByText('app has no running container'),
    ).toBeInTheDocument()
  })
})
