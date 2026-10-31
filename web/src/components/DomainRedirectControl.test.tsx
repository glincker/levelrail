import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainRedirectControl } from './DomainRedirectControl'
import { createTestQueryClient } from '@/test/testQueryClient'

// Enforcement is covered by DomainMaintenanceControl.test.tsx; here the
// control's own states are under test, so the domain counts as enforced.
vi.mock('./domains/EnforcementGate', () => ({
  EnforcedBadge: ({ children }: { children: React.ReactNode }) => children,
  EnforcementNote: () => null,
}))

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
  const queryClient = createTestQueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <DomainRedirectControl appName="demo-app" domain="www.example.com" />
    </QueryClientProvider>,
  )
}

describe('DomainRedirectControl', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows "No redirect" when nothing is configured', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse({
            domain: 'www.example.com',
            enabled: false,
            status_code: 301,
          }),
        ),
      ),
    )

    renderControl()

    expect(await screen.findByText('No redirect')).toBeInTheDocument()
  })

  it('shows the target url badge once a redirect is configured', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse({
            domain: 'www.example.com',
            enabled: true,
            target_url: 'https://example.com',
            status_code: 301,
          }),
        ),
      ),
    )

    renderControl()

    expect(
      await screen.findByText('Redirecting to https://example.com'),
    ).toBeInTheDocument()
  })

  it('PUTs the target_url/status_code fields on save', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input, init) => {
      const url = requestUrlOf(input)
      if (!url.endsWith('/redirect')) {
        throw new Error(`unexpected fetch: ${url}`)
      }
      if (init?.method === 'PUT') {
        return Promise.resolve(
          fakeJsonResponse({
            domain: 'www.example.com',
            enabled: true,
            target_url: 'https://example.com',
            status_code: 301,
          }),
        )
      }
      return Promise.resolve(
        fakeJsonResponse({
          domain: 'www.example.com',
          enabled: false,
          status_code: 301,
        }),
      )
    })
    vi.stubGlobal('fetch', fetchMock)

    renderControl()

    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', { name: /add redirect/i }),
    )
    await user.type(screen.getByLabelText('Target URL'), 'https://example.com')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'PUT',
      )
      expect(putCall).toBeDefined()
      const body = JSON.parse((putCall?.[1]?.body as string) ?? '{}') as {
        target_url: string
        status_code: number
      }
      expect(body.target_url).toBe('https://example.com')
      expect(body.status_code).toBe(301)
    })
  })
})
