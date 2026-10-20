import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ViewInGrafanaLink } from './ViewInGrafanaLink'

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderLink() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ViewInGrafanaLink />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ViewInGrafanaLink', () => {
  it('renders nothing when no external dashboard URL is configured', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse({
          external_dashboard_url: '',
          remote_read_path: '/api/v1/prometheus/read',
        }),
      ),
    )
    renderLink()
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.queryByText('View in Grafana')).not.toBeInTheDocument()
  })

  it('renders nothing while the fetch is still pending', () => {
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(new Promise(() => {})))
    renderLink()
    expect(screen.queryByText('View in Grafana')).not.toBeInTheDocument()
  })

  it('links to the configured external dashboard URL once set', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse({
          external_dashboard_url: 'https://grafana.example.com/d/abc',
          remote_read_path: '/api/v1/prometheus/read',
        }),
      ),
    )
    renderLink()
    const link = await screen.findByRole('link', { name: /View in Grafana/ })
    expect(link).toHaveAttribute('href', 'https://grafana.example.com/d/abc')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noreferrer')
  })

  it('renders nothing when the fetch fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ error: 'boom' }, 500)),
    )
    renderLink()
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.queryByText('View in Grafana')).not.toBeInTheDocument()
  })
})
