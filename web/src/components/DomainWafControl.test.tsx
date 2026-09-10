import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainWafControl } from './DomainWafControl'

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
      <DomainWafControl appName="demo-app" domain="app.example.com" />
    </QueryClientProvider>,
  )
}

describe('DomainWafControl', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows "Not protected" when nothing is configured', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse({
            domain: 'app.example.com',
            waf_enabled: false,
            waf_mode: 'detect',
            rate_limit_enabled: false,
            rate_limit_rps: 0,
            rate_limit_burst: 0,
          }),
        ),
      ),
    )

    renderControl()

    expect(await screen.findByText('Not protected')).toBeInTheDocument()
  })

  it('shows the enabled badge once the WAF is on', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse({
            domain: 'app.example.com',
            waf_enabled: true,
            waf_mode: 'block',
            rate_limit_enabled: false,
            rate_limit_rps: 0,
            rate_limit_burst: 0,
          }),
        ),
      ),
    )

    renderControl()

    expect(await screen.findByText('WAF/rate limit on')).toBeInTheDocument()
  })

  it('PUTs the enabled waf_enabled/waf_mode/rate limit fields on save', async () => {
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      (input, init) => {
        const url = requestUrlOf(input)
        if (!url.endsWith('/waf')) {
          throw new Error(`unexpected fetch: ${url}`)
        }
        if (init?.method === 'PUT') {
          return Promise.resolve(
            fakeJsonResponse({
              domain: 'app.example.com',
              waf_enabled: true,
              waf_mode: 'detect',
              rate_limit_enabled: false,
              rate_limit_rps: 0,
              rate_limit_burst: 0,
            }),
          )
        }
        return Promise.resolve(
          fakeJsonResponse({
            domain: 'app.example.com',
            waf_enabled: false,
            waf_mode: 'detect',
            rate_limit_enabled: false,
            rate_limit_rps: 0,
            rate_limit_burst: 0,
          }),
        )
      },
    )
    vi.stubGlobal('fetch', fetchMock)

    renderControl()

    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /add waf/i }))
    await user.click(screen.getByRole('switch', { name: /owasp coraza waf/i }))
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'PUT',
      )
      expect(putCall).toBeDefined()
      const body = JSON.parse((putCall?.[1]?.body as string) ?? '{}') as {
        waf_enabled: boolean
        waf_mode: string
        rate_limit_rps: number
      }
      expect(body.waf_enabled).toBe(true)
      expect(body.waf_mode).toBe('detect')
      expect(body.rate_limit_rps).toBe(0)
    })
  })
})
