import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { DatabaseResourceLimitsEditor } from './DatabaseResourceLimitsEditor'
import type { DatabaseResource } from '../types/databaseDetail'

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

function wasCalledWith(
  fetchMock: ReturnType<typeof vi.fn>,
  url: string,
  method: string,
): boolean {
  return fetchMock.mock.calls.some(
    (call) =>
      requestUrlOf(call[0] as RequestInfo | URL) === url &&
      ((call[1] as RequestInit | undefined)?.method ?? 'GET') === method,
  )
}

function fakeDatabase(
  overrides: Partial<DatabaseResource> = {},
): DatabaseResource {
  return {
    name: 'demo-db',
    engine: 'postgres',
    version: '16',
    ...overrides,
  }
}

function renderEditor(
  database: DatabaseResource,
  fetchMock: ReturnType<typeof vi.fn>,
) {
  vi.stubGlobal('fetch', fetchMock)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <DatabaseResourceLimitsEditor database={database} />
    </QueryClientProvider>,
  )
}

function nodeAwareFetchMock(putResult: DatabaseResource) {
  return vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = requestUrlOf(input)
    const method = init?.method ?? 'GET'
    if (url === '/api/v1/nodes/node-1') {
      return Promise.resolve(
        fakeJsonResponse({
          id: 'node-1',
          name: 'node-alpha',
          status: 'online',
          schedulable: true,
          accepts_app_workloads: true,
          accepts_build_workloads: true,
          created_at: '2026-01-01T00:00:00Z',
        }),
      )
    }
    if (url.startsWith('/api/v1/nodes/node-1/metrics')) {
      const isMemory = url.includes('metric=memory_usage_bytes')
      return Promise.resolve(
        fakeJsonResponse({
          metric: isMemory ? 'memory_usage_bytes' : 'cpu_percent',
          points: [
            {
              timestamp: '2026-01-01T00:00:00Z',
              value: isMemory ? 268_435_456 : 12.5,
              count: 1,
            },
          ],
          resource_count: 2,
        }),
      )
    }
    if (
      url === '/api/v1/databases/demo-db/resources' &&
      method === 'PUT'
    ) {
      return Promise.resolve(fakeJsonResponse(putResult, 200))
    }
    return Promise.reject(new Error(`unexpected fetch: ${url} ${method}`))
  })
}

describe('DatabaseResourceLimitsEditor', () => {
  it('shows a node capacity hint next to the memory and CPU fields once enabled', async () => {
    const database = fakeDatabase({ node_id: 'node-1' })
    renderEditor(database, nodeAwareFetchMock(database))
    const user = userEvent.setup()

    await user.click(screen.getByRole('switch', { name: 'Memory limit' }))
    await waitFor(() => {
      expect(screen.getByText(/node-alpha/)).toBeInTheDocument()
    })
    expect(screen.getByText(/256.*MiB memory in use/)).toBeInTheDocument()

    await user.click(screen.getByRole('switch', { name: 'CPU limit' }))
    await waitFor(() => {
      expect(screen.getByText(/12\.5% CPU in use/)).toBeInTheDocument()
    })
  })

  it('does not show a hint or block the form when the database has no node assigned', async () => {
    const database = fakeDatabase()
    const fetchMock = nodeAwareFetchMock(database)
    renderEditor(database, fetchMock)
    const user = userEvent.setup()

    await user.click(screen.getByRole('switch', { name: 'Memory limit' }))
    expect(screen.queryByText(/currently has/)).not.toBeInTheDocument()

    await user.type(screen.getByLabelText('Limit (MiB)'), '512')
    await user.click(screen.getByRole('button', { name: /save resource limits/i }))

    await waitFor(() => {
      expect(
        wasCalledWith(fetchMock, '/api/v1/databases/demo-db/resources', 'PUT'),
      ).toBe(true)
    })
  })

  it('still submits successfully when the node capacity hint fails to load', async () => {
    const database = fakeDatabase({ node_id: 'node-1' })
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/nodes/node-1') {
        return Promise.resolve(fakeJsonResponse({ error: 'not found' }, 404))
      }
      if (url.startsWith('/api/v1/nodes/node-1/metrics')) {
        return Promise.resolve(fakeJsonResponse({ error: 'not found' }, 404))
      }
      if (
        url === '/api/v1/databases/demo-db/resources' &&
        method === 'PUT'
      ) {
        return Promise.resolve(fakeJsonResponse(database, 200))
      }
      return Promise.reject(new Error(`unexpected fetch: ${url} ${method}`))
    })
    renderEditor(database, fetchMock)
    const user = userEvent.setup()

    await user.click(screen.getByRole('switch', { name: 'CPU limit' }))
    await user.type(screen.getByLabelText('Limit (cores)'), '0.5')
    await user.click(screen.getByRole('button', { name: /save resource limits/i }))

    await waitFor(() => {
      expect(
        wasCalledWith(fetchMock, '/api/v1/databases/demo-db/resources', 'PUT'),
      ).toBe(true)
    })
    expect(screen.queryByText(/currently has/)).not.toBeInTheDocument()
  })
})
