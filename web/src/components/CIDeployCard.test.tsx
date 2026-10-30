import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CIDeployCard } from './CIDeployCard'
import { createTestQueryClient } from '@/test/testQueryClient'

let root = false
vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => root }))

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

const existing = {
  id: 'ct_1',
  provider: 'gitlab',
  issuer: 'https://gitlab.example.com',
  repository: 'team/web',
  ref_pattern: 'refs/heads/main',
  environment: '',
  protected_ref_only: false,
  created_at: '',
}

function renderCard() {
  const queryClient = createTestQueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={null}>
        <CIDeployCard appName="web" />
      </Suspense>
    </QueryClientProvider>,
  )
}

describe('CIDeployCard providers', () => {
  let posts: Record<string, unknown>[]

  beforeEach(() => {
    root = false
    posts = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        if (url.endsWith('/ci/trusts') && init?.method === 'POST') {
          const body = JSON.parse(init.body as string) as Record<
            string,
            unknown
          >
          posts.push(body)
          return Promise.resolve(jsonResponse({ ...existing, ...body }, 201))
        }
        if (url.endsWith('/ci/trusts')) {
          return Promise.resolve(jsonResponse([existing]))
        }
        if (url.endsWith('/ci/workflow')) {
          return Promise.resolve(
            jsonResponse({
              yaml: 'name: deploy',
              api_url: 'https://cp.example.com',
              has_trust: true,
              needs_dashboard_url: false,
            }),
          )
        }
        return Promise.resolve(jsonResponse({}, 404))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists the provider, issuer and the opt-out warning of a trust', async () => {
    renderCard()
    expect(await screen.findByText('team/web')).toBeTruthy()
    expect(screen.getByText('ciDeploy.issuerShown')).toBeTruthy()
    expect(screen.getAllByText(/ciDeploy.unprotectedWarning/).length).toBe(1)
  })

  it('creates a protected-only GitLab trust and warns when turned off', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('button', { name: 'ciDeploy.providers.gitlab' }),
    )
    expect(screen.queryByText('ciDeploy.issuerLabel')).toBeNull()
    await user.type(
      screen.getByPlaceholderText('ciDeploy.repositoryPlaceholder.gitlab'),
      'acme/web',
    )
    await user.click(screen.getByRole('button', { name: 'ciDeploy.trust' }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toMatchObject({
      provider: 'gitlab',
      repository: 'acme/web',
      protected_ref_only: true,
    })
    expect(posts[0]).not.toHaveProperty('issuer')

    await user.click(screen.getByRole('switch'))
    expect(screen.getAllByText(/ciDeploy.unprotectedWarning/).length).toBe(2)
  })

  it('shows the self-managed issuer field to root only and sends the CircleCI org', async () => {
    root = true
    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('button', { name: 'ciDeploy.providers.gitlab' }),
    )
    expect(screen.getByText('ciDeploy.issuerLabel')).toBeTruthy()

    await user.click(
      screen.getByRole('button', { name: 'ciDeploy.providers.circleci' }),
    )
    expect(screen.queryByText('ciDeploy.issuerLabel')).toBeNull()
    await user.type(
      screen.getByPlaceholderText('ciDeploy.orgIdPlaceholder'),
      'org-1',
    )
    await user.type(
      screen.getByPlaceholderText('ciDeploy.repositoryPlaceholder.circleci'),
      'proj-1',
    )
    await user.click(screen.getByRole('button', { name: 'ciDeploy.trust' }))
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toMatchObject({
      provider: 'circleci',
      org_id: 'org-1',
      repository: 'proj-1',
    })
    expect(screen.getByText(/circleci run oidc get/)).toBeTruthy()
  })
})
