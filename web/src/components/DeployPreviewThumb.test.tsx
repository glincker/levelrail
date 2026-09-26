import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DeployPreviewThumb } from './DeployPreviewThumb'
import type { PreviewRecord, PreviewStatus } from '../types/preview'

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

function statusBody(overrides: Partial<PreviewStatus> = {}): PreviewStatus {
  return {
    app: 'web',
    enabled: true,
    path: '/',
    wait_ms: 0,
    server_enabled: true,
    capturing: false,
    image: 'browser:1',
    keep_per_app: 5,
    ttl_days: 30,
    max_total_mb: 200,
    storage: { app_bytes: 0, app_count: 0, total_bytes: 0, total_count: 0 },
    ...overrides,
  }
}

function record(overrides: Partial<PreviewRecord>): PreviewRecord {
  return {
    deployment_id: 'dep_1',
    status: 'ok',
    path: '/',
    bytes: 100,
    captured_at: '2026-09-01T10:00:00Z',
    ...overrides,
  }
}

function renderThumb(
  props: Partial<Parameters<typeof DeployPreviewThumb>[0]> = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <DeployPreviewThumb appName="web" deploymentId="dep_1" {...props} />
    </QueryClientProvider>,
  )
}

describe('DeployPreviewThumb', () => {
  let history: PreviewRecord[]
  let status: PreviewStatus
  let captured: number
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    history = []
    status = statusBody()
    captured = 0
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = urlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/apps/web/preview/history') {
        return Promise.resolve(jsonResponse(history))
      }
      if (url === '/api/v1/apps/web/preview/capture' && method === 'POST') {
        captured += 1
        return Promise.resolve(
          jsonResponse({ ...status, capturing: true }, 202),
        )
      }
      if (url === '/api/v1/apps/web/preview') {
        return Promise.resolve(jsonResponse(status))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows the image from the deploy history url', async () => {
    renderThumb({ imageUrl: '/api/v1/apps/web/deployments/dep_1/preview?v=1' })
    const img = await screen.findByRole('img', {
      name: 'Preview of deployment dep_1',
    })
    expect(img).toHaveAttribute(
      'src',
      '/api/v1/apps/web/deployments/dep_1/preview?v=1',
    )
  })

  it('explains a skipped capture with its reason', async () => {
    history = [record({ status: 'skipped', reason: 'auth_wall' })]
    renderThumb({ size: 'md' })
    expect(await screen.findByText('No preview')).toBeInTheDocument()
    expect(
      screen.getByText('Skipped: the page needs a login.'),
    ).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('renders nothing for a row with no preview when previews are off', async () => {
    status = statusBody({ enabled: false })
    const { container } = renderThumb({ hideWhenEmpty: true })
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    await waitFor(() => expect(container).toBeEmptyDOMElement())
  })

  it('offers Recapture only for the live release and queues a capture', async () => {
    const user = userEvent.setup()
    renderThumb({ canRecapture: true })
    const button = await screen.findByRole('button', { name: 'Recapture' })
    await user.click(button)
    await waitFor(() => expect(captured).toBe(1))
  })

  it('hides Recapture when previews are not enabled for the app', async () => {
    status = statusBody({ enabled: false })
    renderThumb({ canRecapture: true })
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    expect(screen.queryByRole('button', { name: 'Recapture' })).toBeNull()
  })
})
