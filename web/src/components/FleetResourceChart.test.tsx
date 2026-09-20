import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FleetResourceChart } from './FleetResourceChart'
import type { AppResourceUsage } from '../types/appResourceUsage'

function fakeJsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderChart() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <FleetResourceChart />
    </QueryClientProvider>,
  )
}

describe('FleetResourceChart', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders nothing while telemetry is not configured (501)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(fakeJsonResponse({ error: 'not configured' }, 501)),
      ),
    )
    const { container } = renderChart()

    await waitFor(() => expect(container).toBeEmptyDOMElement())
  })

  it('renders nothing when there are no apps', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse([]))),
    )
    const { container } = renderChart()

    await waitFor(() => expect(container).toBeEmptyDOMElement())
  })

  it('shows total CPU and memory once a resource-usage snapshot arrives', async () => {
    const usage: AppResourceUsage[] = [
      { name: 'api', cpu_percent: 12, memory_usage_bytes: 1024 * 1024 * 100 },
      { name: 'worker', cpu_percent: 8, memory_usage_bytes: 1024 * 1024 * 50 },
    ]
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse(usage))),
    )
    renderChart()

    await screen.findByText('Fleet resource usage')
    expect(screen.getByText('Across 2 apps')).toBeInTheDocument()
    expect(screen.getByText('Total CPU')).toBeInTheDocument()
    expect(screen.getByText('Total memory')).toBeInTheDocument()
    // A single snapshot is one history point, too few to draw a trend
    // line from, so both sparklines show the "collecting" placeholder
    // rather than an empty or misleading chart.
    expect(screen.getAllByText(/collecting data/i)).toHaveLength(2)
  })

  it('reports "no data yet" for a metric no app has ever recorded', async () => {
    const usage: AppResourceUsage[] = [{ name: 'api', cpu_percent: 5 }]
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse(usage))),
    )
    renderChart()

    await screen.findByText('Fleet resource usage')
    expect(screen.getByText('Total memory')).toBeInTheDocument()
    expect(screen.getByText('No data yet.')).toBeInTheDocument()
    expect(screen.getAllByText(/collecting data/i)).toHaveLength(1)
  })
})
