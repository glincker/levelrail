import { Suspense } from 'react'
import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CreateAppFromGitFields } from './CreateAppFromGitFields'

const navigateMock = vi.fn()

// Stubbed so rendering the component doesn't require a real router: only
// useNavigate/Link are ever reached (GitHubAppRepoPicker's "not
// connected" CTA links to /settings/github-app), neither is asserted on
// here.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
    Link: ({
      children,
      to,
      className,
    }: {
      children?: ReactNode
      to?: string
      className?: string
    }) => (
      <a href={to} className={className}>
        {children}
      </a>
    ),
  }
})

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

function renderForm() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  const onCreated = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={<div>loading</div>}>
        <CreateAppFromGitFields open onCreated={onCreated} />
      </Suspense>
    </QueryClientProvider>,
  )
  return { onCreated }
}

describe('CreateAppFromGitFields', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let buildAttempts: number

  beforeEach(() => {
    buildAttempts = 0
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'

      if (url === '/api/v1/github-app' && method === 'GET') {
        return Promise.resolve(
          fakeJsonResponse({ connected: false, installed: false }),
        )
      }
      if (url === '/api/v1/apps' && method === 'POST') {
        return Promise.resolve(
          fakeJsonResponse(
            { name: 'demo-app', image: 'demo-app:pending-build', port: 3000 },
            201,
          ),
        )
      }
      if (url === '/api/v1/apps/demo-app/builds' && method === 'POST') {
        buildAttempts += 1
        // First trigger fails, e.g. a private repo with no credentials
        // configured yet. Second (retried) trigger succeeds.
        if (buildAttempts === 1) {
          return Promise.resolve(
            fakeJsonResponse(
              { error: 'no credentials configured for this repository' },
              502,
            ),
          )
        }
        return Promise.resolve(fakeJsonResponse({ id: 'attempt-1' }, 200))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('unlocks build fields after a failed build trigger and lets the user retry', async () => {
    const user = userEvent.setup()
    const { onCreated } = renderForm()

    await screen.findByLabelText('Name')

    await user.type(screen.getByLabelText('Name'), 'demo-app')
    await user.type(screen.getByLabelText('Port'), '3000')
    await user.type(
      screen.getByLabelText('Repository URL'),
      'https://github.com/example/private-repo.git',
    )
    await user.type(screen.getByLabelText(/Branch, tag, or commit/), 'main')

    await user.click(screen.getByRole('button', { name: 'Build and deploy' }))

    await screen.findByText(/App created, but triggering the build failed/)

    // Identity fields (sent only by POST /api/v1/apps, step 1) stay
    // locked: the app record already exists, and a retry never resends
    // them (see buildInputFrom in CreateAppFromGitFields.tsx).
    expect(screen.getByLabelText('Name')).toBeDisabled()
    expect(screen.getByLabelText('Port')).toBeDisabled()

    // Build/git fields (resent on every retry) must be editable again,
    // otherwise "fix the fields above and submit again" has nothing to
    // act on.
    expect(screen.getByLabelText('Repository URL')).not.toBeDisabled()
    expect(screen.getByLabelText(/Branch, tag, or commit/)).not.toBeDisabled()

    await user.clear(screen.getByLabelText('Repository URL'))
    await user.type(
      screen.getByLabelText('Repository URL'),
      'https://github.com/example/fixed-repo.git',
    )

    await user.click(screen.getByRole('button', { name: 'Retry build' }))

    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledTimes(1)
    })

    // The app record itself is never re-created: only the build retried.
    const createAppCalls = fetchMock.mock.calls.filter(([input, init]) => {
      const requestUrl = requestUrlOf(input as RequestInfo | URL)
      const requestInit = init as RequestInit | undefined
      return requestUrl === '/api/v1/apps' && requestInit?.method === 'POST'
    })
    expect(createAppCalls).toHaveLength(1)
    expect(buildAttempts).toBe(2)
  })
})
