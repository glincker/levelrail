import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GitSourceCard } from './GitSourceCard'
import type { AppDetail } from '../types/appDetail'

// GitRepoSourcePicker itself (its provider status suspense queries, its
// repo/branch Select popups) is covered separately in
// GitRepoSourcePicker.test.tsx, the same split
// CreateAppFromGitFields.test.tsx already establishes for the wizard.
// Mocked here to a set of test-only buttons so these tests can drive
// GitSourceCard's own connect-endpoint choice (provider use-as-source vs
// the generic PUT) and its non-picker behaviors (build pack tabs,
// additional services validation, webhook banner) without opening a real
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
            repoUrl: 'https://github.com/acme/app.git',
            branch: 'main',
            providerRef: { kind: 'github', owner: 'acme', repo: 'app' },
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
            providerRef: { kind: 'gitlab', projectId: 7 },
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
      <button
        type="button"
        onClick={() => {
          onSelect({
            provider: 'manual',
            repoUrl: 'https://example.com/acme/self-hosted.git',
            branch: 'main',
            token: 'tok_abc123',
          })
        }}
      >
        Pick manual URL (test)
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

const app: AppDetail = {
  name: 'demo-app',
  image: 'demo-app:latest',
  port: 3000,
  strategy: 'rolling',
  replicas: 1,
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

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <GitSourceCard app={app} />
    </QueryClientProvider>,
  )
}

describe('GitSourceCard', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let gitSourceStatus: number

  beforeEach(() => {
    gitSourceStatus = 404
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'

      if (url === '/api/v1/apps/demo-app/git-source' && method === 'GET') {
        if (gitSourceStatus === 404) {
          return Promise.resolve(fakeJsonResponse({ error: 'not found' }, 404))
        }
        return Promise.resolve(fakeJsonResponse(fakeGitSourceResource(), 200))
      }
      if (url === '/api/v1/apps/demo-app/git-source' && method === 'PUT') {
        return Promise.resolve(fakeJsonResponse(fakeGitSourceResource(), 201))
      }
      if (
        url === '/api/v1/github-app/repos/acme/app/use-as-source' &&
        method === 'POST'
      ) {
        return Promise.resolve(
          fakeJsonResponse(
            {
              ...fakeGitSourceResource({ repo_url: 'https://github.com/acme/app.git' }),
              webhook_registered: true,
            },
            201,
          ),
        )
      }
      if (
        url === '/api/v1/gitlab-app/projects/7/use-as-source' &&
        method === 'POST'
      ) {
        return Promise.resolve(
          fakeJsonResponse(
            fakeGitSourceResource({ repo_url: 'https://gitlab.example.com/acme/app.git' }),
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
            fakeGitSourceResource({ repo_url: 'https://bitbucket.org/acme/app.git' }),
            201,
          ),
        )
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('connects via the generic endpoint for a manual pick, including the pasted token', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Pick manual URL (test)' }))
    await user.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => {
      expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(1)
    })
    const [call] = callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')
    const body = JSON.parse(call?.init?.body as string) as {
      repo_url: string
      branch: string
      token?: string
    }
    expect(body.repo_url).toBe('https://example.com/acme/self-hosted.git')
    expect(body.token).toBe('tok_abc123')

    // Manual connects never auto-register a webhook: the secret/URL
    // paste-by-hand banner must still show.
    await screen.findByText(/will not be shown again/)
    expect(screen.getByText('wh_secret_abc')).toBeInTheDocument()
  })

  it('connects via use-as-source for a picked GitHub repo, registering the webhook automatically', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Pick GitHub repo (test)' }))
    await user.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => {
      expect(
        callsTo(fetchMock, '/api/v1/github-app/repos/acme/app/use-as-source', 'POST'),
      ).toHaveLength(1)
    })
    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(0)

    await screen.findByText(/registered automatically/)
    expect(screen.queryByText('wh_secret_abc')).not.toBeInTheDocument()
  })

  it('connects via use-as-source for a picked GitLab project', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Pick GitLab project (test)' }))
    await user.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => {
      expect(
        callsTo(fetchMock, '/api/v1/gitlab-app/projects/7/use-as-source', 'POST'),
      ).toHaveLength(1)
    })
    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(0)
  })

  it('connects via use-as-source for a picked Bitbucket repo', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Pick Bitbucket repo (test)' }))
    await user.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => {
      expect(
        callsTo(fetchMock, '/api/v1/bitbucket-app/repos/acme/app/use-as-source', 'POST'),
      ).toHaveLength(1)
    })
    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(0)
  })

  it('falls back to the generic endpoint when additional services are configured alongside a provider pick', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Pick GitHub repo (test)' }))
    await user.click(screen.getByRole('button', { name: 'Add service' }))
    await user.type(screen.getByPlaceholderText('worker'), 'worker')

    await user.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => {
      expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(1)
    })
    expect(
      callsTo(fetchMock, '/api/v1/github-app/repos/acme/app/use-as-source', 'POST'),
    ).toHaveLength(0)

    const [call] = callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')
    const body = JSON.parse(call?.init?.body as string) as {
      repo_url: string
      additional_services?: Record<string, unknown>
    }
    expect(body.repo_url).toBe('https://github.com/acme/app.git')
    expect(body.additional_services).toEqual({
      worker: { build_type: 'dockerfile', build_path: undefined },
    })
  })

  it('blocks save and shows an error when an additional service row has no name', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Pick manual URL (test)' }))
    await user.click(screen.getByRole('button', { name: 'Add service' }))
    await user.type(screen.getByPlaceholderText('./worker/Dockerfile'), './worker')

    await user.click(screen.getByRole('button', { name: 'Connect' }))

    await screen.findByText(/missing a service name/)
    expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(0)
    expect(
      callsTo(fetchMock, '/api/v1/github-app/repos/acme/app/use-as-source', 'POST'),
    ).toHaveLength(0)
  })

  it('sends the chosen build pack and path with a manual connect', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(await screen.findByRole('button', { name: 'Pick manual URL (test)' }))
    await user.click(screen.getByRole('tab', { name: 'Dockerfile' }))
    await user.type(screen.getByLabelText('Dockerfile path (optional)'), './build/Dockerfile')

    await user.click(screen.getByRole('button', { name: 'Connect' }))

    await waitFor(() => {
      expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(1)
    })
    const [call] = callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')
    const body = JSON.parse(call?.init?.body as string) as {
      build_type: string
      build_path?: string
    }
    expect(body.build_type).toBe('dockerfile')
    expect(body.build_path).toBe('./build/Dockerfile')
  })

  it('shows the connected view and lets an existing connection be edited without re-picking a repo', async () => {
    gitSourceStatus = 200
    const user = userEvent.setup()
    renderCard()

    await screen.findByText('https://example.com/repo.git')
    await user.click(screen.getByRole('button', { name: 'Edit' }))

    // Editing shows what stays connected unless a new pick is made.
    await screen.findByText(/Currently connected to/)
    expect(screen.getByText('https://example.com/repo.git', { selector: 'span' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')).toHaveLength(1)
    })
    const [call] = callsTo(fetchMock, '/api/v1/apps/demo-app/git-source', 'PUT')
    const body = JSON.parse(call?.init?.body as string) as { repo_url: string; branch: string }
    // Unchanged repo/branch resent as-is, since no new pick overrode them.
    expect(body.repo_url).toBe('https://example.com/repo.git')
    expect(body.branch).toBe('main')
  })
})
