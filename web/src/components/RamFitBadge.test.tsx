import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RamFitBadge } from './RamFitBadge'

function requestUrlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function fakeJsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function mockFetchRoutes(
  routes: Record<string, () => Promise<Response>>,
): void {
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) => {
      const url = requestUrlOf(input)
      const path = url.split('?')[0] ?? url
      const handler = routes[path]
      if (handler) return handler()
      return Promise.reject(new Error(`unexpected fetch: ${url}`))
    }),
  )
}

function renderBadge(recommendedMemoryBytes: number) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RamFitBadge recommendedMemoryBytes={recommendedMemoryBytes} />
    </QueryClientProvider>,
  )
}

describe('RamFitBadge', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders nothing when no node is local', async () => {
    mockFetchRoutes({
      '/api/v1/nodes': () =>
        Promise.resolve(
          fakeJsonResponse([
            { id: 'node-1', name: 'worker-1', is_local: false },
          ]),
        ),
    })
    renderBadge(6 * 1024 * 1024 * 1024)
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.queryByText(/free/)).not.toBeInTheDocument()
  })

  it('shows a fit badge when the local node has enough available memory', async () => {
    mockFetchRoutes({
      '/api/v1/nodes': () =>
        Promise.resolve(
          fakeJsonResponse([
            { id: 'node-1', name: 'worker-1', is_local: true },
          ]),
        ),
      '/api/v1/nodes/node-1/metrics': () =>
        Promise.resolve(
          fakeJsonResponse({
            metric: 'memory_available_bytes',
            points: [
              {
                timestamp: '2026-01-01T00:00:00Z',
                value: 8 * 1024 * 1024 * 1024,
                count: 1,
              },
            ],
            resource_count: 1,
          }),
        ),
    })
    renderBadge(6 * 1024 * 1024 * 1024)
    expect(await screen.findByText(/Fits on worker-1/)).toBeInTheDocument()
  })

  it('shows a warning badge when the local node does not have enough available memory', async () => {
    mockFetchRoutes({
      '/api/v1/nodes': () =>
        Promise.resolve(
          fakeJsonResponse([
            { id: 'node-1', name: 'worker-1', is_local: true },
          ]),
        ),
      '/api/v1/nodes/node-1/metrics': () =>
        Promise.resolve(
          fakeJsonResponse({
            metric: 'memory_available_bytes',
            points: [
              {
                timestamp: '2026-01-01T00:00:00Z',
                value: 2 * 1024 * 1024 * 1024,
                count: 1,
              },
            ],
            resource_count: 1,
          }),
        ),
    })
    renderBadge(6 * 1024 * 1024 * 1024)
    expect(
      await screen.findByText(/May not fit on worker-1/),
    ).toBeInTheDocument()
  })
})
