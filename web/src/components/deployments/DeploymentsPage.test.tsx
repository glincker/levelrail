import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { makeDeployment } from '../../test/deploymentFixtures'
import { parseDeploymentsSearch } from '../../lib/deploymentFilters'
import { DeploymentsPage } from './DeploymentsPage'
import type { Deployment } from '../../types/deployment'

class FakeEventSource {
  static CLOSED = 2
  readyState = 1
  onopen: (() => void) | null = null
  onmessage: (() => void) | null = null
  onerror: (() => void) | null = null
  close() {}
}

const summary = {
  window: '24h',
  counts: {},
  in_progress: 0,
  needs_attention: 0,
  failure_rate_24h: null,
  duration: { median_ms: null, p95_ms: null, samples: 0 },
  per_day: [],
}

function mockApi(items: Deployment[], listStatus = 200) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const body = url.includes('/summary')
        ? summary
        : { items: url.includes('building') ? [] : items, next_cursor: '' }
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status:
            url.includes('/summary') || url.includes('building')
              ? 200
              : listStatus,
        }),
      )
    }),
  )
}

function renderPage(search: Record<string, unknown> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onSearchChange = vi.fn()
  render(
    <QueryClientProvider client={qc}>
      <DeploymentsPage
        search={parseDeploymentsSearch(search)}
        onSearchChange={onSearchChange}
      />
    </QueryClientProvider>,
  )
  return { onSearchChange }
}

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource)
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DeploymentsPage', () => {
  it('shows a skeleton first, then the rows', async () => {
    mockApi([makeDeployment({ id: 'a' })])
    renderPage()
    expect(
      screen.getByRole('status', { name: 'Loading deployments' }),
    ).toBeInTheDocument()
    expect(
      await screen.findByText('Fix the login redirect'),
    ).toBeInTheDocument()
  })

  it('shows the empty state when nothing has ever deployed', async () => {
    mockApi([])
    renderPage()
    expect(await screen.findByText('No deployments yet')).toBeInTheDocument()
  })

  it('offers to clear filters when nothing matches', async () => {
    mockApi([])
    const { onSearchChange } = renderPage({ status: 'failed' })
    await userEvent.click(
      await screen.findByRole('button', { name: 'Clear filters' }),
    )
    expect(onSearchChange).toHaveBeenCalledWith({})
  })

  it('shows an error state with retry', async () => {
    mockApi([], 500)
    renderPage()
    expect(
      await screen.findByText('Deployments could not be loaded'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('writes the open row into the URL', async () => {
    mockApi([makeDeployment({ id: 'a' })])
    const { onSearchChange } = renderPage()
    await userEvent.click(await screen.findByText('Fix the login redirect'))
    expect(onSearchChange).toHaveBeenCalledWith({ d: 'a' })
  })
})
