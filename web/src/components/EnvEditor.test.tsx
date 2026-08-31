import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EnvEditor } from './EnvEditor'
import type { AppDetail } from '../types/appDetail'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: { children?: ReactNode; to?: string } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
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

const app: AppDetail = {
  name: 'demo-app',
  image: 'demo-app:latest',
  port: 3000,
  strategy: 'rolling',
  replicas: 1,
  suspended: false,
  env: { FOO: 'own-value', SHADOWED: 'own-shadow' },
  project_id: 'proj_1',
  environment_id: 'env_1',
}

function renderEditor() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <EnvEditor app={app} />
    </QueryClientProvider>,
  )
}

describe('EnvEditor tier provenance', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('badges an own key that shadows a higher tier, an own key that does not, and lists a purely inherited key read-only', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = requestUrlOf(input)
        if (url === '/api/v1/projects/proj_1') {
          return Promise.resolve(
            fakeJsonResponse({ id: 'proj_1', name: 'demo', created_at: '2026-01-01T00:00:00Z', org_id: 'org_1' }),
          )
        }
        if (url === '/api/v1/organizations/org_1/env') {
          return Promise.resolve(fakeJsonResponse({ SHADOWED: 'org-value' }))
        }
        if (url === '/api/v1/projects/proj_1/env') {
          return Promise.resolve(fakeJsonResponse({ FROM_PROJECT: 'project-value' }))
        }
        if (url === '/api/v1/environments/env_1/env') {
          return Promise.resolve(fakeJsonResponse({}))
        }
        if (url.startsWith('/api/v1/audit-log')) {
          return Promise.resolve(fakeJsonResponse([]))
        }
        return Promise.reject(new Error(`unexpected fetch: ${url}`))
      }),
    )

    renderEditor()

    // FOO has no higher-tier definition: plain "own value".
    await screen.findByText('own value')
    // SHADOWED is also set at the organization tier: the override badge.
    const badgeTexts = await waitFor(() => {
      const badges = Array.from(
        document.querySelectorAll('[data-slot="badge"]'),
      ).map((el) => el.textContent)
      expect(badges).toContain('own value · overrides organization')
      return badges
    })
    expect(badgeTexts).toContain('own value')
    // FROM_PROJECT isn't in the app's own env at all: shown read-only.
    expect(screen.getByText('FROM_PROJECT')).toBeInTheDocument()
    expect(screen.getByText('project-value')).toBeInTheDocument()
    expect(screen.getByText('from project')).toBeInTheDocument()
  })
})
