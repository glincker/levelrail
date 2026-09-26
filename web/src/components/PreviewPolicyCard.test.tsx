import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PreviewPolicyCard } from './PreviewPolicyCard'
import { ApprovePreviewDialog } from './ApprovePreviewDialog'
import type {
  PreviewEnvironment,
  PreviewPolicy,
} from '../types/previewEnvironment'

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function renderWithClient(node: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(<QueryClientProvider client={queryClient}>{node}</QueryClientProvider>)
}

const gitSource = {
  service_name: 'web',
  repo_url: 'https://example.com/repo.git',
  branch: 'main',
  build_type: 'railpack',
  has_token: false,
  webhook_url: '/api/v1/webhooks/github/web',
  preview_enabled: true,
  post_pr_comments: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

describe('PreviewPolicyCard', () => {
  let policy: PreviewPolicy
  let puts: unknown[]

  beforeEach(() => {
    puts = []
    policy = {
      on_limit: 'evict_oldest',
      allow_fork_previews: false,
      ttl_hours: 0,
      effective_ttl_hours: 168,
      max_per_app: 10,
      live_count: 3,
      max_total: 0,
      live_total: 7,
    }
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        const method = init?.method ?? 'GET'
        if (url === '/api/v1/apps/web/git-source') {
          return Promise.resolve(jsonResponse(gitSource))
        }
        if (url === '/api/v1/apps/web/preview-policy' && method === 'GET') {
          return Promise.resolve(jsonResponse(policy))
        }
        if (url === '/api/v1/apps/web/preview-policy' && method === 'PUT') {
          const body = JSON.parse(
            init?.body as string,
          ) as Partial<PreviewPolicy>
          puts.push(body)
          policy = { ...policy, ...body }
          return Promise.resolve(jsonResponse(policy))
        }
        return Promise.resolve(jsonResponse({ error: 'unexpected' }, 500))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows limit usage and defaults to fork previews off', async () => {
    renderWithClient(<PreviewPolicyCard appName="web" />)

    expect(await screen.findByText('3 live of 10')).toBeInTheDocument()
    expect(screen.getByText('7 live of no limit')).toBeInTheDocument()
    expect(
      screen.getByRole('switch', {
        name: 'Deploy previews for pull requests from forks',
      }),
    ).not.toBeChecked()
  })

  it('saves the fork policy and warns when it is turned on', async () => {
    const user = userEvent.setup()
    renderWithClient(<PreviewPolicyCard appName="web" />)

    await user.click(
      await screen.findByRole('switch', {
        name: 'Deploy previews for pull requests from forks',
      }),
    )

    await waitFor(() => {
      expect(puts).toEqual([{ allow_fork_previews: true }])
    })
    expect(
      await screen.findByText(/Anyone who can open a pull request from a fork/),
    ).toBeInTheDocument()
  })

  it('saves the preview lifetime', async () => {
    const user = userEvent.setup()
    renderWithClient(<PreviewPolicyCard appName="web" />)

    await screen.findByText('3 live of 10')
    const input = screen.getByRole('textbox')
    await user.clear(input)
    await user.type(input, '12')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(puts).toEqual([{ ttl_hours: 12 }])
    })
  })
})

describe('ApprovePreviewDialog', () => {
  const preview: PreviewEnvironment = {
    app_name: 'web',
    pr_number: 42,
    preview_app_id: 'web-pr-42',
    branch: 'feature-x',
    head_sha: 'abc1234',
    status: 'awaiting_approval',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    stale: false,
    is_fork: true,
    head_repo: 'mallory/web',
  }
  let posts: unknown[]

  beforeEach(() => {
    posts = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        if (
          urlOf(input) === '/api/v1/apps/web/previews/42/approve' &&
          init?.method === 'POST'
        ) {
          posts.push(JSON.parse(init.body as string))
          return Promise.resolve(jsonResponse({ status: 'deploying' }, 202))
        }
        return Promise.resolve(jsonResponse({ error: 'unexpected' }, 500))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('states the security implication and only deploys after confirming', async () => {
    const user = userEvent.setup()
    renderWithClient(<ApprovePreviewDialog appName="web" preview={preview} />)

    await user.click(screen.getByRole('button', { name: 'Approve preview' }))
    expect(
      await screen.findByText(/environment variables and secrets/),
    ).toBeInTheDocument()
    expect(screen.getByText(/mallory\/web/)).toBeInTheDocument()
    expect(posts).toEqual([])

    await user.click(screen.getByRole('button', { name: 'Approve and deploy' }))
    await waitFor(() => {
      expect(posts).toEqual([{ confirm: true }])
    })
  })
})
