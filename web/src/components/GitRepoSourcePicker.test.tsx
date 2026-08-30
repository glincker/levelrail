import { Suspense } from 'react'
import type { ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GitRepoSourcePicker } from './GitRepoSourcePicker'
import type { GitProviderStatus } from '../types/gitProviders'

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

function disconnected(provider: GitProviderStatus['provider']): GitProviderStatus {
  return {
    provider,
    connected: false,
    can_list_branches: false,
    can_register_webhook: false,
    can_auth_clone: false,
  }
}

function renderPicker() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const onSelect = vi.fn()
  const result = render(
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={<div>loading</div>}>
        <GitRepoSourcePicker onSelect={onSelect} />
      </Suspense>
    </QueryClientProvider>,
  )
  return { onSelect, container: result.container }
}

// branchFieldById scopes past the always-visible manual "Or paste a
// repository URL" row's own "Branch" input: once a provider row's
// branch control is showing too, screen.getByLabelText('Branch') is
// ambiguous (both the provider row and the manual row use that same
// label text), so tests that need one specific provider's branch
// control look it up by its own field id instead.
function branchFieldById(container: HTMLElement, id: string): HTMLElement {
  const el = container.querySelector(`#${id}`)
  if (!el) throw new Error(`no element with id ${id}`)
  return el as HTMLElement
}

describe('GitRepoSourcePicker', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let providers: GitProviderStatus[]

  beforeEach(() => {
    providers = [disconnected('github'), disconnected('gitlab'), disconnected('bitbucket')]
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/git-providers' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(providers))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    // base-ui's Select sets pointer-events: none on <body> while its
    // popup is open and clears it asynchronously on close; a test that
    // ends right after picking an option can outrun that cleanup, which
    // would otherwise leak into the next test as a "pointer-events: none"
    // element blocking every subsequent user-event interaction.
    document.body.style.pointerEvents = ''
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
    // One aggregated call, not three per-provider status calls.
    const statusCalls = fetchMock.mock.calls.filter(
      ([input]) => requestUrlOf(input as RequestInfo | URL) === '/api/v1/git-providers',
    )
    expect(statusCalls).toHaveLength(1)
  })

  it('shows the repository picker once GitHub is connected', async () => {
    providers = [
      { provider: 'github', connected: true, can_list_branches: true, can_register_webhook: true, can_auth_clone: true },
      disconnected('gitlab'),
      disconnected('bitbucket'),
    ]
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/git-providers' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(providers))
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
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPicker()

    expect(await screen.findByText('Repository')).toBeInTheDocument()
    // GitLab and Bitbucket stay collapsed to their "Not connected" CTA.
    expect(screen.getAllByText('Connect')).toHaveLength(2)
  })

  it('emits a github providerRef once a repo and branch are picked', async () => {
    providers = [
      { provider: 'github', connected: true, can_list_branches: true, can_register_webhook: true, can_auth_clone: true },
      disconnected('gitlab'),
      disconnected('bitbucket'),
    ]
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/git-providers' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(providers))
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
      if (url === '/api/v1/github-app/repos/acme/app/branches' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse([{ name: 'main', commit_sha: 'abc123' }]))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    const { onSelect, container } = renderPicker()

    // fireEvent, not user.click, for these Select interactions: base-ui's
    // Select applies pointer-events:none while positioning its popup, and
    // that inline style can still be present the instant an option
    // renders, which user.click's real-interaction pointer-events guard
    // treats as unclickable. This suite only needs to prove the wiring
    // (which onSelect payload a pick produces), not real pointer
    // accessibility, so fireEvent.click sidesteps that guard.
    fireEvent.click(await screen.findByLabelText('Repository'))
    fireEvent.click(await screen.findByText('acme/app'))
    fireEvent.click(branchFieldById(container, 'git-picker-github-branch'))
    fireEvent.click(await screen.findByText('main'))

    expect(onSelect).toHaveBeenLastCalledWith({
      provider: 'github',
      repoUrl: 'https://github.com/acme/app.git',
      branch: 'main',
      providerRef: { kind: 'github', owner: 'acme', repo: 'app' },
    })
  })

  it('shows a real branch select for a connected GitLab project, not the old free-text fallback', async () => {
    providers = [
      disconnected('github'),
      { provider: 'gitlab', connected: true, can_list_branches: true, can_register_webhook: true, can_auth_clone: false },
      disconnected('bitbucket'),
    ]
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/git-providers' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(providers))
      }
      if (url === '/api/v1/gitlab-app/projects' && method === 'GET') {
        return Promise.resolve(
          fakeJsonResponse([
            {
              id: 7,
              name: 'web',
              path_with_namespace: 'acme/web',
              clone_url: 'https://gitlab.example.com/acme/web.git',
              default_branch: 'main',
              visibility: 'private',
              web_url: 'https://gitlab.example.com/acme/web',
            },
          ]),
        )
      }
      if (url === '/api/v1/gitlab-app/projects/7/branches' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse([{ name: 'main', commit_sha: 'abc' }, { name: 'dev', commit_sha: 'def' }]))
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    const { onSelect, container } = renderPicker()

    fireEvent.click(await screen.findByLabelText('Project'))
    fireEvent.click(await screen.findByText('acme/web'))

    // Picking the project alone already emits its default branch, the
    // same immediate-pick shape GitLabProviderRow always had.
    expect(onSelect).toHaveBeenLastCalledWith({
      provider: 'gitlab',
      repoUrl: 'https://gitlab.example.com/acme/web.git',
      branch: 'main',
      providerRef: { kind: 'gitlab', projectId: 7 },
    })

    // The branch control is now a combobox (Select) fed by real branch
    // data, not the old free-text fallback: the "no branch-listing API
    // here yet" copy is gone, and both fetched branches are listed once
    // the control is opened.
    expect(screen.queryByText(/branch-listing/i)).not.toBeInTheDocument()
    const branchControl = branchFieldById(container, 'git-picker-gitlab-branch')
    expect(branchControl).toHaveAttribute('role', 'combobox')

    fireEvent.click(branchControl)
    expect(await screen.findByText('dev')).toBeInTheDocument()
  })

  it('falls back to a free-text branch field when a connected GitLab project cannot list branches', async () => {
    providers = [
      disconnected('github'),
      { provider: 'gitlab', connected: true, can_list_branches: false, can_register_webhook: true, can_auth_clone: false },
      disconnected('bitbucket'),
    ]
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/git-providers' && method === 'GET') {
        return Promise.resolve(fakeJsonResponse(providers))
      }
      if (url === '/api/v1/gitlab-app/projects' && method === 'GET') {
        return Promise.resolve(
          fakeJsonResponse([
            {
              id: 7,
              name: 'web',
              path_with_namespace: 'acme/web',
              clone_url: 'https://gitlab.example.com/acme/web.git',
              default_branch: 'main',
              visibility: 'private',
              web_url: 'https://gitlab.example.com/acme/web',
            },
          ]),
        )
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)

    const { container } = renderPicker()

    fireEvent.click(await screen.findByLabelText('Project'))
    fireEvent.click(await screen.findByText('acme/web'))

    const branchControl = branchFieldById(container, 'git-picker-gitlab-branch')
    expect(branchControl).not.toHaveAttribute('role', 'combobox')
    expect(branchControl).toHaveValue('main')
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
