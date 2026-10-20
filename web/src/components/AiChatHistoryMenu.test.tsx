import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Suspense } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AiChatHistoryMenu } from './AiChatHistoryMenu'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderWithClient(node: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <Suspense fallback="loading">{node}</Suspense>
    </QueryClientProvider>,
  )
}

describe('AiChatHistoryMenu', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('lists sessions and resumes the chosen one', async () => {
    const user = userEvent.setup()
    const onResume = vi.fn()
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input) => {
      if (urlOf(input) === '/api/v1/ai/sessions') {
        return Promise.resolve(
          json([
            {
              id: 'aisess_1',
              created_at: '2026-10-01T00:00:00Z',
              updated_at: '2026-10-01T00:05:00Z',
            },
          ]),
        )
      }
      throw new Error(`unexpected fetch: ${urlOf(input)}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithClient(
      <AiChatHistoryMenu activeSessionId={null} onResume={onResume} />,
    )

    await user.click(await screen.findByRole('button', { name: /history/i }))
    const resumeItem = await screen.findByText(/resume session/i)
    await user.click(resumeItem)

    expect(onResume).toHaveBeenCalledWith('aisess_1')
  })

  it('deletes a session and refetches the list', async () => {
    const user = userEvent.setup()
    let sessions = [
      {
        id: 'aisess_1',
        created_at: '2026-10-01T00:00:00Z',
        updated_at: '2026-10-01T00:05:00Z',
      },
    ]
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input, init) => {
      if (urlOf(input) === '/api/v1/ai/sessions') {
        return Promise.resolve(json(sessions))
      }
      if (
        urlOf(input) === '/api/v1/ai/sessions/aisess_1' &&
        init?.method === 'DELETE'
      ) {
        sessions = []
        return Promise.resolve({ ok: true, status: 204 } as unknown as Response)
      }
      throw new Error(`unexpected fetch: ${urlOf(input)} ${init?.method}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithClient(
      <AiChatHistoryMenu activeSessionId={null} onResume={vi.fn()} />,
    )

    await user.click(await screen.findByRole('button', { name: /history/i }))
    await user.click(await screen.findByText(/delete this session/i))

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([input, init]) =>
            urlOf(input) === '/api/v1/ai/sessions/aisess_1' &&
            init?.method === 'DELETE',
        ),
      ).toBe(true)
    })
  })
})
