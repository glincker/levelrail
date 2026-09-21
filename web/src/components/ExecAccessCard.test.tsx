import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ExecAccessCard } from './ExecAccessCard'

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

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ExecAccessCard appName="demo-app" />
    </QueryClientProvider>,
  )
}

describe('ExecAccessCard', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let execEnabled: boolean

  beforeEach(() => {
    // On by default, the one place this toggle deliberately differs
    // from AutoRollbackCard's own opt-in fixture default.
    execEnabled = true
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'

      if (url === '/api/v1/apps/demo-app/exec-access' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse({ enabled: execEnabled }, 200))
      }
      if (url === '/api/v1/apps/demo-app/exec-access' && method === 'PUT') {
        const body = JSON.parse(init?.body as string) as { enabled: boolean }
        execEnabled = body.enabled
        return Promise.resolve(fakeJsonResponse({ enabled: execEnabled }, 200))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('loads on, matching the default-true server behavior', async () => {
    renderCard()

    const toggle = await screen.findByRole('switch', {
      name: 'Shell/exec access enabled',
    })
    expect(toggle).toHaveAttribute('aria-checked', 'true')
  })

  it('disables exec access on click and persists it via PUT', async () => {
    const user = userEvent.setup()
    renderCard()

    const toggle = await screen.findByRole('switch', {
      name: 'Shell/exec access enabled',
    })
    await user.click(toggle)

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([input, init]) =>
            requestUrlOf(input as RequestInfo) ===
              '/api/v1/apps/demo-app/exec-access' &&
            (init as RequestInit)?.method === 'PUT',
        ),
      ).toBe(true)
    })

    await waitFor(() => {
      expect(toggle).toHaveAttribute('aria-checked', 'false')
    })
  })
})
