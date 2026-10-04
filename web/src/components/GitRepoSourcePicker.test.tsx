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
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
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

function disconnected(
  provider: GitProviderStatus['provider'],
): GitProviderStatus {
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
  account_type: 'organization' as const,
}

const fakeGiteaRepo = {
  full_name: 'acme/app',
  name: 'app',
  private: false,
  default_branch: 'main',
  clone_url: 'https://git.example.com/acme/app.git',
  web_url: 'https://git.example.com/acme/app',
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
async function branchFieldById(
  container: HTMLElement,
  id: string,
): Promise<HTMLElement> {
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

// pickComboboxOption is pickOption's twin for the branch Combobox
// specifically: a repo card's own default-branch chip (GitRepoCard, e.g.
// "main") can share text with the real branch option once the combobox
// opens, so getByText alone is ambiguous there. Scoping to role="option"
// (the Combobox's own option buttons, not the chip's plain span) resolves
// it without needing distinct fixture branch names.
async function pickComboboxOption(
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
      fireEvent.click(screen.getByRole('option', { name: optionText }))
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
    providers = [
      disconnected('github'),
      disconnected('gitlab'),
      disconnected('bitbucket'),
      disconnected('gitea'),
    ]
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': () =>
        Promise.resolve(fakeJsonResponse(providers)),
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

  it("defaults to the GitHub tab with a Connect prompt when no provider is connected, and switching tabs shows each provider's own Connect prompt", async () => {
    renderPicker()

    // GitHub is the first tab, so it's the fallback active tab when
    // nothing is connected: its own Connect prompt shows by default, the
    // other three providers' prompts aren't in the active panel yet.
    expect(
      await screen.findByText('Connect GitHub to pick a repository.'),
    ).toBeInTheDocument()
    expect(screen.queryByText('Repository')).not.toBeInTheDocument()
    expect(screen.queryByText('Project')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Connect' })).toHaveAttribute(
      'href',
      '/settings/github-app',
    )

    fireEvent.click(screen.getByRole('tab', { name: /GitLab/i }))
    expect(
      await screen.findByText('Connect GitLab to pick a repository.'),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Connect' })).toHaveAttribute(
      'href',
      '/settings/gitlab-app',
    )

    fireEvent.click(screen.getByRole('tab', { name: /Bitbucket/i }))
    expect(
      await screen.findByText('Connect Bitbucket to pick a repository.'),
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: /Gitea/i }))
    expect(
      await screen.findByText('Connect Gitea to pick a repository.'),
    ).toBeInTheDocument()

    // One aggregated call, not three per-provider status calls.
    const statusCalls = fetchMock.mock.calls.filter(
      ([input]) =>
        requestUrlOf(input as RequestInfo | URL) === '/api/v1/git-providers',
    )
    expect(statusCalls).toHaveLength(1)
  })

  it('shows the repository picker once GitHub is connected, and defaults the tab strip to it', async () => {
    providers = [
      {
        provider: 'github',
        connected: true,
        can_list_branches: true,
        can_register_webhook: true,
        can_auth_clone: true,
      },
      disconnected('gitlab'),
      disconnected('bitbucket'),
      disconnected('gitea'),
    ]
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': jsonRoute(providers),
      'GET /api/v1/github-app/repos': jsonRoute({ repos: [fakeGitHubRepo] }),
    })

    renderPicker()

    // Connected provider wins the default-active tab over the first tab
    // in list order, even though both happen to be GitHub here.
    expect(await screen.findByText('Repository')).toBeInTheDocument()
    expect(screen.queryByText('Connect GitHub')).not.toBeInTheDocument()

    // GitLab stays collapsed to its own Connect prompt until its tab is picked.
    fireEvent.click(screen.getByRole('tab', { name: /GitLab/i }))
    expect(
      await screen.findByText('Connect GitLab to pick a repository.'),
    ).toBeInTheDocument()
  })

  it('emits a github providerRef once a repo and branch are picked', async () => {
    providers = [
      {
        provider: 'github',
        connected: true,
        can_list_branches: true,
        can_register_webhook: true,
        can_auth_clone: true,
      },
      disconnected('gitlab'),
      disconnected('bitbucket'),
      disconnected('gitea'),
    ]
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': jsonRoute(providers),
      'GET /api/v1/github-app/repos': jsonRoute({ repos: [fakeGitHubRepo] }),
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
    await pickComboboxOption(
      container,
      'git-picker-github-branch',
      'main',
      () => {
        expect(onSelect).toHaveBeenCalled()
      },
    )

    expect(onSelect).toHaveBeenLastCalledWith({
      provider: 'github',
      repoUrl: 'https://github.com/acme/app.git',
      branch: 'main',
      providerRef: { kind: 'github', owner: 'acme', repo: 'app' },
    })
  })

  it('groups GitHub repos by connected account and filters the grid via chips, without hiding another account on a listing error', async () => {
    providers = [
      {
        provider: 'github',
        connected: true,
        can_list_branches: true,
        can_register_webhook: true,
        can_auth_clone: true,
      },
      disconnected('gitlab'),
      disconnected('bitbucket'),
      disconnected('gitea'),
    ]
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': jsonRoute(providers),
      'GET /api/v1/github-app/repos': jsonRoute({
        repos: [
          fakeGitHubRepo,
          {
            full_name: 'acme-person/dotfiles',
            name: 'dotfiles',
            owner_login: 'acme-person',
            private: true,
            default_branch: 'main',
            clone_url: 'https://github.com/acme-person/dotfiles.git',
            account_type: 'user',
          },
        ],
        errors: [
          { account_login: 'suspended-org', error: 'installation suspended' },
        ],
      }),
    })

    renderPicker()
    await screen.findByText('Repository')

    // Three accounts (two with repos, one suspended): the "All" chip
    // plus one per account, not the single ungrouped grid a lone
    // account would get. Chips only mount once the repos fetch itself
    // resolves, so this waits rather than asserting immediately.
    expect(
      await screen.findByRole('button', { name: 'All' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'acme' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Personal (acme-person)' }),
    ).toBeInTheDocument()
    // "suspended-org" appears twice (its filter chip and its own group
    // header), so this checks presence via getAllByText rather than the
    // single-match getByText the other two account names use above.
    expect(screen.getAllByText('suspended-org').length).toBeGreaterThan(0)
    expect(screen.getByText('installation suspended')).toBeInTheDocument()

    // All accounts' repos show at once by default.
    expect(screen.getByText('acme/app')).toBeInTheDocument()
    expect(screen.getByText('acme-person/dotfiles')).toBeInTheDocument()

    // Picking the "acme" chip narrows the grid to that account alone.
    fireEvent.click(screen.getByRole('button', { name: 'acme' }))
    expect(screen.getByText('acme/app')).toBeInTheDocument()
    expect(screen.queryByText('acme-person/dotfiles')).not.toBeInTheDocument()
  })

  it('shows a real branch select for a connected GitLab project, not the old free-text fallback', async () => {
    providers = [
      disconnected('github'),
      {
        provider: 'gitlab',
        connected: true,
        can_list_branches: true,
        can_register_webhook: true,
        can_auth_clone: false,
      },
      disconnected('bitbucket'),
      disconnected('gitea'),
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
    const branchControl = await branchFieldById(
      container,
      'git-picker-gitlab-branch',
    )
    expect(branchControl).toHaveAttribute('role', 'combobox')

    fireEvent.click(branchControl)
    expect(await screen.findByText('dev')).toBeInTheDocument()
  })

  it('falls back to a free-text branch field when a connected GitLab project cannot list branches', async () => {
    providers = [
      disconnected('github'),
      {
        provider: 'gitlab',
        connected: true,
        can_list_branches: false,
        can_register_webhook: true,
        can_auth_clone: false,
      },
      disconnected('bitbucket'),
      disconnected('gitea'),
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

    const branchControl = await branchFieldById(
      container,
      'git-picker-gitlab-branch',
    )
    expect(branchControl).not.toHaveAttribute('role', 'combobox')
    expect(branchControl).toHaveValue('main')
  })

  it('emits a manual pick once a pasted URL and branch are both filled in', async () => {
    const user = userEvent.setup()
    const { onSelect } = renderPicker()

    await screen.findByText('Connect GitHub to pick a repository.')
    fireEvent.click(screen.getByRole('tab', { name: /URL/i }))

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

    await screen.findByText('Connect GitHub to pick a repository.')
    fireEvent.click(screen.getByRole('tab', { name: /URL/i }))

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

  it('emits a gitea providerRef once a repo and branch are picked', async () => {
    providers = [
      disconnected('github'),
      disconnected('gitlab'),
      disconnected('bitbucket'),
      {
        provider: 'gitea',
        connected: true,
        can_list_branches: true,
        can_register_webhook: true,
        can_auth_clone: false,
      },
    ]
    fetchMock = mockFetchRoutes({
      'GET /api/v1/git-providers': jsonRoute(providers),
      'GET /api/v1/gitea-app/repos': jsonRoute([fakeGiteaRepo]),
      'GET /api/v1/gitea-app/repos/acme/app/branches': jsonRoute([
        { name: 'main', commit_sha: 'abc123' },
      ]),
    })

    const { onSelect, container } = renderPicker()
    await screen.findByLabelText('Repository')

    await pickOption(container, 'git-picker-gitea-repo', 'acme/app', () => {
      expect(container.querySelector('#git-picker-gitea-branch')).toBeTruthy()
    })
    await pickComboboxOption(
      container,
      'git-picker-gitea-branch',
      'main',
      () => {
        expect(onSelect).toHaveBeenCalled()
      },
    )

    expect(onSelect).toHaveBeenLastCalledWith({
      provider: 'gitea',
      repoUrl: 'https://git.example.com/acme/app.git',
      branch: 'main',
      providerRef: { kind: 'gitea', owner: 'acme', repo: 'app' },
    })
  })
})
