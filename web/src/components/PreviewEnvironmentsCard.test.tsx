import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PreviewEnvironmentsCard } from './PreviewEnvironmentsCard'
import type { AppDetail } from '../types/appDetail'

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

function fakeGitSourceResource(overrides: Record<string, unknown> = {}) {
  return {
    service_name: 'demo-app',
    repo_url: 'https://example.com/repo.git',
    branch: 'main',
    build_type: 'railpack',
    has_token: false,
    webhook_url: '/api/v1/webhooks/github/demo-app',
    preview_enabled: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const app: AppDetail = {
  name: 'demo-app',
  image: 'demo-app:latest',
  port: 3000,
  strategy: 'rolling',
  replicas: 1,
  suspended: false,
  env_dirty: false,
}

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <PreviewEnvironmentsCard app={app} />
    </QueryClientProvider>,
  )
}

describe('PreviewEnvironmentsCard', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let previewEnabled: boolean
  let gitSourceConnected: boolean

  beforeEach(() => {
    previewEnabled = false
    gitSourceConnected = true
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'

      if (url === '/api/v1/apps/demo-app/git-source' && method === 'GET') {
        if (!gitSourceConnected) {
          return Promise.resolve(fakeJsonResponse({ error: 'not found' }, 404))
        }
        return Promise.resolve(fakeJsonResponse(fakeGitSourceResource({ preview_enabled: previewEnabled }), 200))
      }
      if (url === '/api/v1/apps/demo-app/preview-settings' && method === 'PUT') {
        const body = JSON.parse(init?.body as string) as { enabled: boolean }
        previewEnabled = body.enabled
        return Promise.resolve(fakeJsonResponse({ enabled: body.enabled }, 200))
      }
      if (url === '/api/v1/apps/demo-app/previews' && method === 'GET') {
        return Promise.resolve(
          fakeJsonResponse(
            previewEnabled
              ? [
                  {
                    pr_number: 42,
                    preview_app_id: 'demo-app-pr-42',
                    branch: 'feature-x',
                    head_sha: 'abc123',
                    domain: 'pr-42.demo-app.example.com',
                    status: 'active',
                    created_at: '2026-01-01T00:00:00Z',
                    updated_at: '2026-01-01T00:00:00Z',
                  },
                ]
              : [],
            200,
          ),
        )
      }
      if (url === '/api/v1/apps/demo-app/previews/42/teardown' && method === 'POST') {
        return Promise.resolve(fakeJsonResponse(null, 200))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('disables the toggle until a git source is connected', async () => {
    gitSourceConnected = false
    renderCard()

    const toggle = await screen.findByRole('switch', { name: 'Preview environments enabled' })
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
  })

  it('enables previews and shows the active preview list', async () => {
    const user = userEvent.setup()
    renderCard()

    const toggle = await screen.findByRole('switch', { name: 'Preview environments enabled' })
    await waitFor(() => {
      expect(toggle).not.toHaveAttribute('aria-disabled', 'true')
    })

    await user.click(toggle)

    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([input, init]) =>
        requestUrlOf(input as RequestInfo) === '/api/v1/apps/demo-app/preview-settings' && (init as RequestInit)?.method === 'PUT',
      )).toBe(true)
    })

    await screen.findByText('PR #42')
    expect(screen.getByText('feature-x')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /pr-42\.demo-app\.example\.com/ })).toHaveAttribute(
      'href',
      'https://pr-42.demo-app.example.com',
    )
  })

  it('tears down a preview on button click', async () => {
    previewEnabled = true
    const user = userEvent.setup()
    renderCard()

    await screen.findByText('PR #42')
    await user.click(screen.getByRole('button', { name: /tear down/i }))

    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([input, init]) =>
        requestUrlOf(input as RequestInfo) === '/api/v1/apps/demo-app/previews/42/teardown' && (init as RequestInit)?.method === 'POST',
      )).toBe(true)
    })
  })
})
