import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainErrorPagesControl } from './DomainErrorPagesControl'

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

function renderControl() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <DomainErrorPagesControl appName="demo-app" domain="app.example.com" />
    </QueryClientProvider>,
  )
}

describe('DomainErrorPagesControl', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows "No custom error pages" when nothing is configured', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse({ domain: 'app.example.com', pages: [] }),
        ),
      ),
    )

    renderControl()

    expect(await screen.findByText('No custom error pages')).toBeInTheDocument()
  })

  it('shows the configured-count badge once pages exist', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse({
            domain: 'app.example.com',
            pages: [{ status_code: 404, body: '<h1>not found</h1>' }],
          }),
        ),
      ),
    )

    renderControl()

    expect(await screen.findByText('1 custom error page')).toBeInTheDocument()
  })

  it('PUTs the status_code/body fields on save', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input, init) => {
      const url = requestUrlOf(input)
      if (!url.includes('/error-pages')) {
        throw new Error(`unexpected fetch: ${url}`)
      }
      if (init?.method === 'PUT') {
        return Promise.resolve(
          fakeJsonResponse({
            domain: 'app.example.com',
            pages: [{ status_code: 404, body: '<h1>not found</h1>' }],
          }),
        )
      }
      return Promise.resolve(
        fakeJsonResponse({ domain: 'app.example.com', pages: [] }),
      )
    })
    vi.stubGlobal('fetch', fetchMock)

    renderControl()

    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', { name: /add error page/i }),
    )
    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: '404' }))
    await user.type(screen.getByLabelText('Custom HTML'), '<h1>not found</h1>')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'PUT',
      )
      expect(putCall).toBeDefined()
      const body = JSON.parse((putCall?.[1]?.body as string) ?? '{}') as {
        status_code: number
        body: string
      }
      expect(body.status_code).toBe(404)
      expect(body.body).toBe('<h1>not found</h1>')
    })
  })

  it('DELETEs with the status_code query param when removing one page', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input, init) => {
      const url = requestUrlOf(input)
      if (init?.method === 'DELETE') {
        expect(url).toContain('status_code=404')
        return Promise.resolve(
          fakeJsonResponse({ domain: 'app.example.com', pages: [] }),
        )
      }
      return Promise.resolve(
        fakeJsonResponse({
          domain: 'app.example.com',
          pages: [{ status_code: 404, body: '<h1>not found</h1>' }],
        }),
      )
    })
    vi.stubGlobal('fetch', fetchMock)

    renderControl()

    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /manage/i }))
    await user.click(await screen.findByRole('button', { name: 'Remove' }))

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([, init]) => init?.method === 'DELETE'),
      ).toBe(true)
    })
  })
})
