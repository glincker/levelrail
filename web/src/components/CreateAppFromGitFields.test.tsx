import { Suspense } from 'react'
import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CreateAppFromGitFields } from './CreateAppFromGitFields'

const navigateMock = vi.fn()

// Stubbed so rendering the component doesn't require a real router: only
// useNavigate/Link are ever reached (GitRepoSourcePicker's "not connected"
// CTAs link to each provider's settings page), neither is asserted on
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

// GitRepoSourcePicker itself (its provider status suspense queries, its
// repo/branch Select popups) is covered separately in
// GitRepoSourcePicker.test.tsx. Mocked here to a trio of test-only
// buttons so these tests can drive CreateAppFromGitFields' own submit
// sequencing (create app -> connect git source -> trigger build, the
// fix for both bugs docs-local/research/git-provider-connect-ux-
// unification-proposal.md documents) without needing to open a real
// Select popup in jsdom.
vi.mock('./GitRepoSourcePicker', () => ({
  GitRepoSourcePicker: ({
    onSelect,
  }: {
    onSelect: (value: {
      provider: string
      repoUrl: string
      branch: string
      token?: string
      providerRef?: unknown
    }) => void
  }) => (
    <div>
      <button
        type="button"
        onClick={() => {
          onSelect({
            provider: 'github',
            repoUrl: 'https://github.com/example/gh-repo.git',
            branch: 'main',
          })
        }}
      >
        Pick GitHub repo (test)
      </button>
      <button
        type="button"
        onClick={() => {
          onSelect({
            provider: 'gitlab',
            repoUrl: 'https://gitlab.example.com/acme/app.git',
            branch: 'main',
            providerRef: { kind: 'gitlab', projectId: 42 },
          })
        }}
      >
        Pick GitLab project (test)
      </button>
      <button
        type="button"
        onClick={() => {
          onSelect({
            provider: 'bitbucket',
            repoUrl: 'https://bitbucket.org/acme/app.git',
            branch: 'main',
            providerRef: { kind: 'bitbucket', workspace: 'acme', repoSlug: 'app' },
          })
        }}
      >
        Pick Bitbucket repo (test)
      </button>
    </div>
  ),
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

function fakeGitSourceResource(overrides: Record<string, unknown> = {}) {
  return {
    service_name: 'demo-app',
    repo_url: 'https://example.com/repo.git',
    branch: 'main',
    build_type: 'railpack',
    has_token: false,
    webhook_url: '/api/v1/webhooks/git/demo-app',
    webhook_secret: 'wh_secret_abc',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
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

function callsTo(
  fetchMock: ReturnType<typeof vi.fn>,
  url: string,
  method: string,
): { input: RequestInfo | URL; init?: RequestInit }[] {
  return fetchMock.mock.calls
    .map(([input, init]: [RequestInfo | URL, RequestInit | undefined]) => ({ input, init }))
    .filter(
      ({ input, init }) =>
        requestUrlOf(input) === url && (init?.method ?? 'GET') === method,
    )
}

describe('CreateAppFromGitFields', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let buildAttempts: number
  // Only the "locked fields after a failed build" test needs the first
  // build attempt to fail; every other test wants a first-try success so
  // it can assert on the resulting navigation/toast without also
  // exercising the retry path.
  let failFirstBuild: boolean

  beforeEach(() => {
    buildAttempts = 0
    failFirstBuild = false
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'

      if (url === '/api/v1/apps' && method === 'POST') {
        return Promise.resolve(
          fakeJsonResponse(
            { name: 'demo-app', image: 'demo-app:pending-build', port: 3000 },
            201,
          ),
        )
      }
      if (url === '/api/v1/apps/demo-app/git-source' && method === 'PUT') {
        return Promise.resolve(fakeJsonResponse(fakeGitSourceResource(), 201))
      }
      if (
        url === '/api/v1/gitlab-app/projects/42/use-as-source' &&
        method === 'POST'
      ) {
        return Promise.resolve(
          fakeJsonResponse(
            fakeGitSourceResource({
              repo_url: 'https://gitlab.example.com/acme/app.git',
            }),
            201,
          ),
        )
      }
      if (
        url === '/api/v1/bitbucket-app/repos/acme/app/use-as-source' &&
        method === 'POST'
      ) {
        return Promise.resolve(
          fakeJsonResponse(
            fakeGitSourceResource({
              repo_url: 'https://bitbucket.org/acme/app.git',
            }),
            201,
          ),
        )
      }
      if (url === '/api/v1/apps/demo-app/builds' && method === 'POST') {
        buildAttempts += 1
        if (failFirstBuild && buildAttempts === 1) {
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
    failFirstBuild = true
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
    expect(callsTo(fetchMock, '/api/v1/apps', 'POST')).toHaveLength(1)
    expect(buildAttempts).toBe(2)

    // The git source connect call (generic endpoint, no provider picked)
    // only ever fires once: it already succeeded on the first submit, so
    // the retry (which only resubmits the build) must not register a
    // second webhook.
    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(1)
  })

  it('connects the git source via the generic endpoint for a GitHub pick, then triggers the build', async () => {
    const user = userEvent.setup()
    const { onCreated } = renderForm()

    await screen.findByLabelText('Name')
    await user.click(screen.getByRole('button', { name: 'Pick GitHub repo (test)' }))
    await user.type(screen.getByLabelText('Port'), '3000')

    await user.click(screen.getByRole('button', { name: 'Build and deploy' }))

    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledTimes(1)
    })

    // Bug 1 (docs-local/research/git-provider-connect-ux-unification-
    // proposal.md): the GitHub wizard path used to create the app and
    // trigger a build without ever creating a git_source row, so no
    // webhook could ever match a future push. GitHub has no dedicated
    // use-as-source route yet (that's PR 2), so this must degrade to the
    // generic connect endpoint, but it must still happen.
    const connectCalls = callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')
    expect(connectCalls).toHaveLength(1)
    const body = JSON.parse(connectCalls[0]?.init?.body as string) as {
      repo_url: string
      branch: string
    }
    expect(body.repo_url).toBe('https://github.com/example/gh-repo.git')
    expect(body.branch).toBe('main')

    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/builds', 'POST')).toHaveLength(1)
  })

  it('connects the git source via use-as-source for a GitLab pick, then triggers the build', async () => {
    const user = userEvent.setup()
    const { onCreated } = renderForm()

    await screen.findByLabelText('Name')
    await user.click(screen.getByRole('button', { name: 'Pick GitLab project (test)' }))
    await user.type(screen.getByLabelText('Port'), '3000')

    await user.click(screen.getByRole('button', { name: 'Build and deploy' }))

    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledTimes(1)
    })

    // Bug 2: the GitLab settings-page path wires the git source and
    // webhook correctly but never triggered a first build. Here it's
    // reached from the wizard instead, so both the connect call and the
    // build call must happen.
    const connectCalls = callsTo(
      fetchMock,
      '/api/v1/gitlab-app/projects/42/use-as-source',
      'POST',
    )
    expect(connectCalls).toHaveLength(1)
    const body = JSON.parse(connectCalls[0]?.init?.body as string) as {
      app_name: string
      branch: string
    }
    expect(body.app_name).toBe('demo-app')
    expect(body.branch).toBe('main')

    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/builds', 'POST')).toHaveLength(1)
    // The generic endpoint must not also be called: GitLab has its own
    // dedicated connect route.
    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(0)
  })

  it('connects the git source via use-as-source for a Bitbucket pick, then triggers the build', async () => {
    const user = userEvent.setup()
    const { onCreated } = renderForm()

    await screen.findByLabelText('Name')
    await user.click(screen.getByRole('button', { name: 'Pick Bitbucket repo (test)' }))
    await user.type(screen.getByLabelText('Port'), '3000')

    await user.click(screen.getByRole('button', { name: 'Build and deploy' }))

    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledTimes(1)
    })

    const connectCalls = callsTo(
      fetchMock,
      '/api/v1/bitbucket-app/repos/acme/app/use-as-source',
      'POST',
    )
    expect(connectCalls).toHaveLength(1)
    const body = JSON.parse(connectCalls[0]?.init?.body as string) as {
      app_name: string
      branch: string
    }
    expect(body.app_name).toBe('demo-app')
    expect(body.branch).toBe('main')

    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/builds', 'POST')).toHaveLength(1)
    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(0)
  })
})
