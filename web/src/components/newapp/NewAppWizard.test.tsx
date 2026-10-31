import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { NewAppWizard } from './NewAppWizard'
import { jsonResponse, nextAnalysis } from '../../test/newAppFixtures'
import { NewAppI18n } from '../../test/newAppI18n'
import type { ImportAnalysis } from '../../queries/importAnalyze'
import type { RepoPreview } from '../../queries/repoPreview'
import type { Brand } from '../../types/brand'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('../../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: '',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: '',
    DiscussionsURL: '',
    RepoURL: '',
  }),
}))

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 56,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 56,
      })),
  }),
}))

const navigate = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigate,
    Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
      <a href={to}>{children}</a>
    ),
  }
})

interface ApiOptions {
  providers?: unknown[]
  analysis?: ImportAnalysis
  analyzeStatus?: number
  preview?: Partial<RepoPreview>
}

function mockApi(opts: ApiOptions = {}) {
  const calls: { key: string; body: unknown }[] = []
  vi.spyOn(globalThis, 'fetch').mockImplementation((input, init) => {
    const method = init?.method ?? 'GET'
    const url = urlOf(input)
    const key = `${method} ${url}`
    const body: unknown =
      typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
    calls.push({ key, body })
    switch (key) {
      case 'GET /api/v1/git-providers':
        return Promise.resolve(
          jsonResponse(
            opts.providers ?? [
              {
                provider: 'github',
                connected: true,
                can_list_branches: true,
                can_register_webhook: true,
                can_auth_clone: true,
              },
            ],
          ),
        )
      case 'GET /api/v1/git/repos?limit=100':
        return Promise.resolve(
          jsonResponse({
            provider: '',
            total: 2,
            loaded: 2,
            limit: 100,
            offset: 0,
            next_offset: null,
            has_more: false,
            truncated: false,
            cap: 10000,
            cached_at: new Date().toISOString(),
            stale: false,
            refreshing: false,
            owners: [],
            connections: [],
            items: [
              {
                key: 'acme/shop',
                full_name: 'acme/shop',
                name: 'shop',
                owner: 'acme',
                private: true,
                archived: false,
                fork: false,
                default_branch: 'main',
                clone_url: 'https://github.com/acme/shop.git',
                account_type: 'organization',
                language: 'TypeScript',
                pushed_at: new Date().toISOString(),
                provider: 'github',
                connection_id: 'github:1',
                account: 'acme',
              },
              {
                key: 'acme/docs',
                full_name: 'acme/docs',
                name: 'docs',
                owner: 'acme',
                private: false,
                archived: false,
                fork: false,
                default_branch: 'main',
                clone_url: 'https://github.com/acme/docs.git',
                account_type: 'organization',
                language: 'MDX',
                provider: 'github',
                connection_id: 'github:1',
                account: 'acme',
              },
            ],
          }),
        )
      case 'POST /api/v1/git-sources/preview':
        return Promise.resolve(
          jsonResponse({
            kind: 'public',
            host: 'github.com',
            repo_url: 'https://github.com/acme/shop',
            owner: 'acme',
            name: 'shop',
            default_branch: 'main',
            branches: ['main', 'dev'],
            visibility: 'public',
            analysis: opts.analysis ?? nextAnalysis,
            ...opts.preview,
          }),
        )
      case 'GET /api/v1/apps':
        return Promise.resolve(jsonResponse([]))
      case 'POST /api/v1/imports/analyze':
        return Promise.resolve(
          jsonResponse(
            opts.analysis ?? nextAnalysis,
            opts.analyzeStatus ?? 200,
          ),
        )
      case 'GET /api/v1/databases/shop-postgres':
        return Promise.resolve(jsonResponse({ error: 'nope' }, 404))
      case 'POST /api/v1/apps/shop/builds':
        return Promise.resolve(jsonResponse({ id: 'att9' }, 202))
      default:
        return Promise.resolve(jsonResponse(body ?? {}, 201))
    }
  })
  return calls
}

function renderWizard() {
  const client = createTestQueryClient()
  return render(
    <NewAppI18n>
      <QueryClientProvider client={client}>
        <NewAppWizard />
      </QueryClientProvider>
    </NewAppI18n>,
  )
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  return input instanceof URL ? input.href : input.url
}

// Warm the lazy chunk so its first-use compile is not charged to a findBy.
beforeAll(async () => {
  await import('./import/RepoImportFlow')
})

afterEach(() => {
  vi.restoreAllMocks()
  navigate.mockReset()
})

describe('NewAppWizard', () => {
  it('previews a picked repo, then deploys from the short form', async () => {
    const user = userEvent.setup()
    const calls = mockApi()
    renderWizard()

    await user.click(await screen.findByRole('button', { name: /acme\/shop/ }))

    expect(await screen.findByLabelText('App name')).toHaveValue('shop')
    expect(screen.getByText(/Detected: Next.js/)).toBeInTheDocument()
    expect(screen.getByLabelText('NEXTAUTH_SECRET')).toHaveAttribute(
      'type',
      'password',
    )
    expect(
      screen.getByText(/Builds with Railpack from main and deploys to/),
    ).toBeInTheDocument()
    const preview = calls.find(
      (c) => c.key === 'POST /api/v1/git-sources/preview',
    )
    expect(preview?.body).toMatchObject({
      repo_url: 'https://github.com/acme/shop.git',
    })
    expect(calls.some((c) => c.key === 'POST /api/v1/imports/analyze')).toBe(
      false,
    )

    await user.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(
      screen.getByText('This variable is needed to start the app.'),
    ).toBeInTheDocument()
    expect(navigate).not.toHaveBeenCalled()

    await user.type(screen.getByLabelText('NEXTAUTH_SECRET'), 'topsecret')
    await user.click(screen.getByRole('button', { name: 'Deploy' }))

    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith({
        to: '/apps/$name/deploys/$deployId/logs',
        params: { name: 'shop', deployId: 'att9' },
      })
    })
    expect(calls.map((c) => c.key)).toEqual(
      expect.arrayContaining([
        'POST /api/v1/databases',
        'POST /api/v1/apps',
        'POST /api/v1/apps/shop/connections',
        'POST /api/v1/apps/shop/builds',
      ]),
    )
  })

  it('shows a connect call to action when no provider is connected', async () => {
    mockApi({ providers: [] })
    renderWizard()
    expect(
      await screen.findByText('Connect a git provider'),
    ).toBeInTheDocument()
    const links = screen.getAllByRole('link', { name: 'Connect' })
    expect(links.map((l) => l.getAttribute('href'))).toEqual([
      '/settings/github-app',
      '/settings/gitlab-app',
      '/settings/bitbucket-app',
      '/settings/gitea-app',
    ])
    expect(screen.getByLabelText('Repository')).toBeInTheDocument()
  })

  it('filters the repository list by search', async () => {
    const user = userEvent.setup()
    mockApi()
    renderWizard()
    await screen.findByRole('button', { name: /acme\/shop/ })
    await user.type(screen.getByLabelText('Repository'), 'mdx')
    expect(screen.queryByRole('button', { name: /acme\/shop/ })).toBeNull()
    expect(
      screen.getByRole('button', { name: /acme\/docs/ }),
    ).toBeInTheDocument()
  })

  it('degrades to the classic analysis when the preview is unavailable', async () => {
    const user = userEvent.setup()
    mockApi({
      providers: [],
      analysis: { error: 'denied' } as never,
      analyzeStatus: 422,
      preview: { kind: 'rate_limited', analysis: undefined },
    })
    renderWizard()
    await user.type(
      await screen.findByLabelText('Repository'),
      'https://github.com/acme/private',
    )
    expect(
      await screen.findByText(/Preview unavailable right now/),
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    expect(await screen.findByText('denied')).toBeInTheDocument()
    expect(screen.getByLabelText('Repository')).toHaveValue(
      'https://github.com/acme/private',
    )
  })

  it('offers an app picker for a monorepo and re-analyzes with the choice', async () => {
    const user = userEvent.setup()
    const mono: ImportAnalysis = {
      ...nextAnalysis,
      monorepo: true,
      build: {
        type: 'dockerfile',
        dockerfile_path: 'apps/web/Dockerfile',
        evidence: 'found Dockerfile at apps/web/Dockerfile: turbo prune',
        alternatives: [
          {
            build_type: 'dockerfile',
            dockerfile_path: 'apps/web/Dockerfile',
            reason: 'turbo prune',
            reason_code: 'turbo_prune',
            recommended: true,
          },
          {
            build_type: 'dockerfile',
            dockerfile_path: 'apps/api/Dockerfile',
            reason: 'turbo prune',
            reason_code: 'turbo_prune',
            recommended: false,
          },
        ],
      },
    }
    const calls = mockApi({
      analysis: mono,
      preview: { kind: 'unsupported_host', analysis: undefined },
    })
    renderWizard()
    await user.type(
      await screen.findByLabelText('Repository'),
      'https://github.com/acme/shop',
    )
    await user.click(await screen.findByRole('button', { name: 'Continue' }))
    expect(
      await screen.findByText(/found Dockerfile at apps\/web\/Dockerfile/),
    ).toBeInTheDocument()
    await user.click(screen.getByLabelText('App in this repository'))
    await user.click(
      await screen.findByRole('option', { name: 'apps/api/Dockerfile' }),
    )
    await waitFor(() => {
      const analyzeCalls = calls.filter(
        (c) => c.key === 'POST /api/v1/imports/analyze',
      )
      expect(analyzeCalls.at(-1)?.body).toMatchObject({
        dockerfile_path: 'apps/api/Dockerfile',
        build_type: 'dockerfile',
      })
    })
  })
})
