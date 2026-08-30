import { Suspense } from 'react'
import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

// mockFetchRoutes stubs global fetch from a "METHOD url" -> response
// table, so each test only ever states which endpoints it needs and
// what they return, not a fresh vi.fn implementation reimplementing the
// same "look up url+method, else reject" dispatch every time.
function mockFetchRoutes(
  routes: Record<string, () => Promise<Response>>,
): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = requestUrlOf(input)
    const method = init?.method ?? 'GET'
    const handler = routes[`${method} ${url}`]
    if (handler) return handler()
    return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function jsonRoute(body: unknown, status = 200): () => Promise<Response> {
  return () => Promise.resolve(fakeJsonResponse(body, status))
}

const fakeGitHubRepo = {
  full_name: 'acme/app',
  name: 'app',
  owner_login: 'acme',
  private: false,
  default_branch: 'main',
  clone_url: 'https://github.com/acme/app.git',
}

const fakeGitLabProject = {
  id: 7,
  name: 'web',
  path_with_namespace: 'acme/web',
  clone_url: 'https://gitlab.example.com/acme/web.git',
  default_branch: 'main',
  visibility: 'private',
  web_url: 'https://gitlab.example.com/acme/web',
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
// control look it up by its own field id instead. Polls via waitFor,
// not a plain querySelector: the field only mounts after a repo/project
// pick's own state update and (for GitLab) an enabled-flag flip commit,
// which doesn't necessarily land in the same microtask fireEvent.click
// flushes, especially under concurrent test-file load.
async function branchFieldById(container: HTMLElement, id: string): Promise<HTMLElement> {
  return waitFor(() => {
    const el = container.querySelector(`#${id}`)
    if (!el) throw new Error(`no element with id ${id}`)
    return el as HTMLElement
  })
}

// pickOption opens the base-ui Select at triggerId and clicks the
// option matching optionText, retrying the open+click pair up to 5
// times on a real setTimeout delay (not testing-library's own waitFor):
// base-ui occasionally drops a synthetic click sent in the same tick a
// popup opens (observed as the popup staying open, aria-expanded stuck
// true, under concurrent test-file load), and waitFor's own
// MutationObserver-driven immediate retries turned that into a
// synchronous busy loop here (the click toggles the popup open/closed
// on every attempt, which is itself a DOM mutation, so waitFor kept
// re-invoking the callback with no delay and never reached its own
// timeout). A small number of real-clock-spaced attempts converges on
// the same "keep trying until it lands" behavior without that failure
// mode. `settled` is the actual assertion the pick should have
// produced (e.g. a new field mounting, or onSelect having been called).
async function pickOption(
  container: HTMLElement,
  triggerId: string,
  optionText: string,
  settled: () => void,
) {
  const trigger = container.querySelector(`#${triggerId}`)
  if (!trigger) throw new Error(`no trigger with id ${triggerId}`)
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(trigger)
    try {
      fireEvent.click(screen.getByText(optionText))
      settled()
      return
    } catch (err) {
      if (attempt === maxAttempts) throw err
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
}

describe('GitRepoSourcePicker', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let providers: GitProviderStatus[]

  beforeEach(() => {
    providers = [disconnected('github'), disconnected('gitlab'), disconnected('bitbucket')]
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': () => Promise.resolve(fakeJsonResponse(providers)),
    })
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
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': jsonRoute(providers),
      'GET /api/v1/github-app/repos': jsonRoute([fakeGitHubRepo]),
    })

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
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': jsonRoute(providers),
      'GET /api/v1/github-app/repos': jsonRoute([fakeGitHubRepo]),
      'GET /api/v1/github-app/repos/acme/app/branches': jsonRoute([
        { name: 'main', commit_sha: 'abc123' },
      ]),
    })

    const { onSelect, container } = renderPicker()
    await screen.findByLabelText('Repository')

    // fireEvent (inside pickOption), not user.click, for these Select
    // interactions: base-ui's Select applies pointer-events:none while
    // positioning its popup, and that inline style can still be present
    // the instant an option renders, which user.click's real-interaction
    // pointer-events guard treats as unclickable. This suite only needs
    // to prove the wiring (which onSelect payload a pick produces), not
    // real pointer accessibility.
    await pickOption(container, 'git-picker-github-repo', 'acme/app', () => {
      expect(container.querySelector('#git-picker-github-branch')).toBeTruthy()
    })
    await pickOption(container, 'git-picker-github-branch', 'main', () => {
      expect(onSelect).toHaveBeenCalled()
    })

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
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': jsonRoute(providers),
      'GET /api/v1/gitlab-app/projects': jsonRoute([fakeGitLabProject]),
      'GET /api/v1/gitlab-app/projects/7/branches': jsonRoute([
        { name: 'main', commit_sha: 'abc' },
        { name: 'dev', commit_sha: 'def' },
      ]),
    })

    const { onSelect, container } = renderPicker()
    await screen.findByLabelText('Project')

    await pickOption(container, 'git-picker-gitlab-project', 'acme/web', () => {
      expect(onSelect).toHaveBeenCalled()
    })

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
    const branchControl = await branchFieldById(container, 'git-picker-gitlab-branch')
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
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': jsonRoute(providers),
      'GET /api/v1/gitlab-app/projects': jsonRoute([fakeGitLabProject]),
    })

    const { container } = renderPicker()
    await screen.findByLabelText('Project')

    await pickOption(container, 'git-picker-gitlab-project', 'acme/web', () => {
      expect(container.querySelector('#git-picker-gitlab-branch')).toBeTruthy()
    })

    const branchControl = await branchFieldById(container, 'git-picker-gitlab-branch')
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
