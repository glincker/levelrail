import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { NodeCapacityHint } from './NodeCapacityHint'
import type { NodeResource } from '../types/nodeDetail'

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

function fakeNode(overrides: Partial<NodeResource> = {}): NodeResource {
  return {
    id: 'node-1',
    name: 'node-alpha',
    status: 'online',
    schedulable: true,
    accepts_app_workloads: true,
    accepts_build_workloads: true,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function renderHint(
  props: { nodeId?: string; dimension: 'memory' | 'cpu' },
  fetchMock: ReturnType<typeof vi.fn>,
) {
  vi.stubGlobal('fetch', fetchMock)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <NodeCapacityHint {...props} />
    </QueryClientProvider>,
  )
}

function metricFetchMock({
  memoryPoints = [{ timestamp: '2026-01-01T00:00:00Z', value: 536_870_912, count: 1 }],
  cpuPoints = [{ timestamp: '2026-01-01T00:00:00Z', value: 42.5, count: 1 }],
  nodeStatus = 200,
  metricStatus = 200,
}: {
  memoryPoints?: { timestamp: string; value: number; count: number }[]
  cpuPoints?: { timestamp: string; value: number; count: number }[]
  nodeStatus?: number
  metricStatus?: number
} = {}) {
  return vi.fn((input: RequestInfo | URL) => {
    const url = requestUrlOf(input)
    if (url === '/api/v1/nodes/node-1') {
      return Promise.resolve(
        nodeStatus === 200
          ? fakeJsonResponse(fakeNode(), 200)
          : fakeJsonResponse({ error: 'not found' }, nodeStatus),
      )
    }
    if (url.startsWith('/api/v1/nodes/node-1/metrics')) {
      if (metricStatus !== 200) {
        return Promise.resolve(fakeJsonResponse({ error: 'nope' }, metricStatus))
      }
      const isMemory = url.includes('metric=memory_usage_bytes')
      return Promise.resolve(
        fakeJsonResponse({
          metric: isMemory ? 'memory_usage_bytes' : 'cpu_percent',
          points: isMemory ? memoryPoints : cpuPoints,
          resource_count: 3,
        }),
      )
    }
    return Promise.reject(new Error(`unexpected fetch: ${url}`))
  })
}

describe('NodeCapacityHint', () => {
  it('renders a memory usage hint with real data', async () => {
    renderHint({ nodeId: 'node-1', dimension: 'memory' }, metricFetchMock())
    await waitFor(() => {
      expect(screen.getByText(/node-alpha/)).toBeInTheDocument()
    })
    expect(screen.getByText(/512.*MiB memory in use/)).toBeInTheDocument()
    expect(screen.getByText(/across 3 services/)).toBeInTheDocument()
  })

  it('renders a CPU usage hint with real data', async () => {
    renderHint({ nodeId: 'node-1', dimension: 'cpu' }, metricFetchMock())
    await waitFor(() => {
      expect(screen.getByText(/42\.5% CPU in use/)).toBeInTheDocument()
    })
  })

  it('renders nothing when no node is assigned', () => {
    const fetchMock = vi.fn()
    renderHint({ nodeId: undefined, dimension: 'memory' }, fetchMock)
    expect(screen.queryByText(/currently has/)).not.toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('renders nothing when the node cannot be resolved', async () => {
    renderHint(
      { nodeId: 'node-1', dimension: 'memory' },
      metricFetchMock({ nodeStatus: 404 }),
    )
    await waitFor(() => {
      expect(screen.queryByText(/currently has/)).not.toBeInTheDocument()
    })
  })

  it('renders nothing when telemetry is not configured', async () => {
    renderHint(
      { nodeId: 'node-1', dimension: 'memory' },
      metricFetchMock({ metricStatus: 501 }),
    )
    await waitFor(() => {
      expect(screen.queryByText(/currently has/)).not.toBeInTheDocument()
    })
  })

  it('renders nothing when the metric series has no points', async () => {
    renderHint(
      { nodeId: 'node-1', dimension: 'memory' },
      metricFetchMock({ memoryPoints: [] }),
    )
    await waitFor(() => {
      expect(screen.queryByText(/currently has/)).not.toBeInTheDocument()
    })
  })
})
