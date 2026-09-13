import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PauseResumeProjectButton } from './PauseResumeProjectButton'

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

function renderButton() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <PauseResumeProjectButton id="proj_1" name="demo" />
    </QueryClientProvider>,
  )
}

describe('PauseResumeProjectButton', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders both a stop and a start action', () => {
    renderButton()
    expect(
      screen.getByRole('button', { name: /stop project/i }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /start project/i }),
    ).toBeInTheDocument()
  })

  it('posts to the stop endpoint when "Stop project" is clicked', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      () =>
        Promise.resolve(
          fakeJsonResponse({
            succeeded_apps: ['web'],
            succeeded_databases: ['main'],
          }),
        ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderButton()

    await user.click(screen.getByRole('button', { name: /stop project/i }))

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(requestUrlOf(url)).toBe('/api/v1/projects/proj_1/stop')
    expect((init as RequestInit).method).toBe('POST')
  })

  it('posts to the start endpoint when "Start project" is clicked', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      () =>
        Promise.resolve(
          fakeJsonResponse({ succeeded_apps: [], succeeded_databases: [] }),
        ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderButton()

    await user.click(screen.getByRole('button', { name: /start project/i }))

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(requestUrlOf(url)).toBe('/api/v1/projects/proj_1/start')
    expect((init as RequestInit).method).toBe('POST')
  })

  it('does not throw when the stop request fails', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse({ error: 'boom' }, 500))),
    )
    renderButton()

    await user.click(screen.getByRole('button', { name: /stop project/i }))

    expect(
      screen.getByRole('button', { name: /stop project/i }),
    ).toBeInTheDocument()
  })
})
