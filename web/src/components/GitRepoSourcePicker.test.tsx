import { Suspense } from 'react'
import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GitRepoSourcePicker } from './GitRepoSourcePicker'

// Same rationale as CreateAppFromGitFields.test.tsx's identical mock:
// only Link's `to` prop matters here (the "Connect" deep links), no real
// router is needed to render it.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
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

function renderPicker() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const onSelect = vi.fn()
  render(
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={<div>loading</div>}>
        <GitRepoSourcePicker onSelect={onSelect} />
      </Suspense>
    </QueryClientProvider>,
  )
  return { onSelect }
}

describe('GitRepoSourcePicker', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let githubStatus: unknown
  let gitlabStatus: unknown
  let bitbucketStatus: unknown

  beforeEach(() => {
    githubStatus = { connected: false, installed: false }
    gitlabStatus = { connected: false, authorized: false }
    bitbucketStatus = { connected: false, authorized: false }
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/github-app' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(githubStatus))
      }
      if (url === '/api/v1/gitlab-app' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(gitlabStatus))
      }
      if (url === '/api/v1/bitbucket-app' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(bitbucketStatus))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows a Connect link for every provider when none are connected, and no repo pickers', async () => {
    renderPicker()

    expect(await screen.findAllByText('Connect')).toHaveLength(3)
    expect(screen.queryByText('Repository')).not.toBeInTheDocument()
    expect(screen.queryByText('Project')).not.toBeInTheDocument()

    const links = screen.getAllByText('Connect')
    const hrefs = links.map((link) => link.getAttribute('href'))
    expect(hrefs).toEqual(
      expect.arrayContaining([
        '/settings/github-app',
        '/settings/gitlab-app',
        '/settings/bitbucket-app',
      ]),
    )
  })

  it('shows the repository picker once GitHub is connected', async () => {
    githubStatus = { connected: true, installed: true }
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/github-app' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(githubStatus))
      }
      if (url === '/api/v1/github-app/repos' && method === 'GET') {
        return Promise.resolve(
          fakeJsonResponse([
            {
              full_name: 'acme/app',
              name: 'app',
              owner_login: 'acme',
              private: false,
              default_branch: 'main',
              clone_url: 'https://github.com/acme/app.git',
            },
          ]),
        )
      }
      if (url === '/api/v1/gitlab-app' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(gitlabStatus))
      }
      if (url === '/api/v1/bitbucket-app' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(bitbucketStatus))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPicker()

    expect(await screen.findByText('Repository')).toBeInTheDocument()
    // GitLab and Bitbucket stay collapsed to their "Not connected" CTA.
    expect(screen.getAllByText('Connect')).toHaveLength(2)
  })

  it('emits a manual pick once a pasted URL and branch are both filled in', async () => {
    const user = userEvent.setup()
    const { onSelect } = renderPicker()

    await screen.findAllByText('Connect')

    await user.type(
      screen.getByLabelText('Paste a repository URL'),
      'https://example.com/acme/app.git',
    )
    expect(onSelect).not.toHaveBeenCalled()

    await user.type(screen.getByLabelText('Branch'), 'main')

    expect(onSelect).toHaveBeenCalledWith({
      provider: 'manual',
      repoUrl: 'https://example.com/acme/app.git',
      branch: 'main',
      token: undefined,
    })
  })

  it('includes the optional deploy token in a manual pick once typed', async () => {
    const user = userEvent.setup()
    const { onSelect } = renderPicker()

    await screen.findAllByText('Connect')

    await user.type(
      screen.getByLabelText('Paste a repository URL'),
      'https://example.com/acme/app.git',
    )
    await user.type(screen.getByLabelText('Branch'), 'main')
    await user.type(
      screen.getByLabelText('Deploy token (optional, for a private repo)'),
      'tok_abc123',
    )

    expect(onSelect).toHaveBeenLastCalledWith({
      provider: 'manual',
      repoUrl: 'https://example.com/acme/app.git',
      branch: 'main',
      token: 'tok_abc123',
    })
  })
})
