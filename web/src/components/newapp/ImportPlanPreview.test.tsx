import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ImportPlanPreview } from './ImportPlanPreview'
import { NewAppI18n } from '../../test/newAppI18n'
import type { RepoPlan } from '../../queries/importRepoPlan'
import { createTestQueryClient } from '@/test/testQueryClient'

const source = {
  provider: 'manual' as const,
  repoUrl: 'https://github.com/acme/mono',
  branch: 'main',
  token: 'ghp_secret',
}

const plan: RepoPlan = {
  repo_url: source.repoUrl,
  branch: 'main',
  package_cap: 60,
  packages_found: 2,
  packages: [
    {
      name: 'mono-web',
      package_name: 'web',
      directory: 'apps/web',
      port: 3000,
      build_kind: 'buildpack',
      watch_paths: [],
      depends_on: ['packages/ui'],
      warnings: [{ code: 'no_dockerfile', message: 'no Dockerfile' }],
    },
    {
      name: 'mono-api',
      package_name: 'api',
      directory: 'apps/api',
      port: null,
      build_kind: 'dockerfile',
      watch_paths: [],
      depends_on: [],
      warnings: [],
    },
  ],
  warnings: [],
  totals: { packages: 2, warnings: 1, flagged: 1 },
}

function mount() {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <NewAppI18n>
        <ImportPlanPreview source={source} />
      </NewAppI18n>
    </QueryClientProvider>,
  )
}

function mockFetch(res: Promise<Response>) {
  const fn = vi.fn((url: string) =>
    url.includes('/imports/repo/plan')
      ? res
      : Promise.resolve(Response.json([])),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

async function openPlan() {
  await userEvent.click(
    screen.getByRole('button', { name: /preview the plan/i }),
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ImportPlanPreview', () => {
  it('fetches nothing until asked, then renders the table', async () => {
    const fetchFn = mockFetch(Promise.resolve(Response.json(plan)))
    mount()
    expect(fetchFn).not.toHaveBeenCalled()
    await openPlan()
    expect(await screen.findByDisplayValue('mono-web')).toBeInTheDocument()
    expect(screen.getByText('apps/api')).toBeInTheDocument()
    expect(screen.getByDisplayValue('3000')).toBeInTheDocument()
    expect(screen.getByText(/No Dockerfile here/)).toBeInTheDocument()
    expect(
      fetchFn.mock.calls.filter(([url]) =>
        String(url).includes('/imports/repo/plan'),
      ),
    ).toHaveLength(1)
  })

  it('shows the loading state while the plan is built', async () => {
    mockFetch(new Promise<Response>(() => undefined))
    mount()
    await openPlan()
    expect(await screen.findByRole('status')).toHaveTextContent(
      /reading every package/i,
    )
  })

  it('shows an error with a retry button', async () => {
    mockFetch(
      Promise.resolve(Response.json({ error: 'boom' }, { status: 502 })),
    )
    mount()
    await openPlan()
    expect(
      await screen.findByText(/could not build the plan/i),
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /try again/i }),
      ).toBeInTheDocument()
    })
  })

  it('shows the empty state when the server sends null lists', async () => {
    mockFetch(
      Promise.resolve(
        Response.json({
          ...plan,
          packages: null,
          warnings: null,
          totals: { packages: 0, warnings: 0, flagged: 0 },
        }),
      ),
    )
    mount()
    await openPlan()
    expect(await screen.findByText(/nothing to plan/i)).toBeInTheDocument()
  })
})
