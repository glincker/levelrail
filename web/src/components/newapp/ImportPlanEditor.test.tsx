import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ImportPlanEditor } from './ImportPlanEditor'
import { NewAppI18n } from '../../test/newAppI18n'
import type { RepoPlan } from '../../queries/importRepoPlan'
import type {
  ImportApplyRequest,
  ImportApplyResponse,
} from '../../queries/importRepoApply'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    params,
  }: {
    children: ReactNode
    params: { name: string }
  }) => <a href={`/apps/${params.name}`}>{children}</a>,
}))

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
      depends_on: ['apps/api'],
      warnings: [],
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
  totals: { packages: 2, warnings: 0, flagged: 0 },
}

function response(
  over: Partial<ImportApplyResponse>,
  results: ImportApplyResponse['results'],
): ImportApplyResponse {
  const counts: Record<string, number> = {}
  for (const r of results) counts[r.status] = (counts[r.status] ?? 0) + 1
  return {
    repo_url: plan.repo_url,
    branch: 'main',
    dry_run: false,
    needs_confirmation: false,
    confirm_above: 10,
    counts,
    results,
    ...over,
  }
}

interface Call {
  url: string
  body: ImportApplyRequest | null
}

function mockFetch(apply: (body: ImportApplyRequest) => ImportApplyResponse) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const body =
        typeof init?.body === 'string'
          ? (JSON.parse(init.body) as ImportApplyRequest)
          : null
      calls.push({ url, body })
      if (url.includes('/imports/repo/apply') && body) {
        return Promise.resolve(Response.json(apply(body)))
      }
      return Promise.resolve(Response.json([]))
    }),
  )
  return calls
}

function mount() {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <NewAppI18n>
        <ImportPlanEditor plan={plan} source={source} />
      </NewAppI18n>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ImportPlanEditor', () => {
  it('edits name, port and env, excludes a package and sends only the rest to the dry run', async () => {
    const calls = mockFetch((b) =>
      response(
        { dry_run: true },
        b.packages.map((p) => ({
          package: p.package,
          name: p.name,
          directory: p.directory,
          status: 'would_create' as const,
        })),
      ),
    )
    mount()
    const user = userEvent.setup()
    await user.clear(screen.getByLabelText('App name for apps/web'))
    await user.type(screen.getByLabelText('App name for apps/web'), 'shop-web')
    await user.type(screen.getByLabelText('Port for mono-api'), '8080')
    await user.click(screen.getByRole('checkbox', { name: 'Include mono-api' }))

    await user.click(
      screen
        .getAllByRole('button', { name: /0 variables/i })
        .at(0) as HTMLElement,
    )
    const dialog = await screen.findByRole('dialog')
    await user.click(
      within(dialog).getByRole('button', { name: /add variable/i }),
    )
    await user.type(within(dialog).getByLabelText('Name'), 'API_KEY')
    await user.type(within(dialog).getByLabelText('Value'), 'abc')
    await user.click(within(dialog).getByRole('checkbox', { name: 'Secret' }))
    await user.click(within(dialog).getByRole('button', { name: 'Done' }))

    await user.click(screen.getByRole('button', { name: /check 1 app/i }))
    expect(
      await screen.findByText(/1 apps can be created/i),
    ).toBeInTheDocument()
    const sent = calls.find((c) => c.body?.dry_run)?.body
    expect(sent?.packages).toHaveLength(1)
    expect(sent?.packages[0]).toMatchObject({
      name: 'shop-web',
      directory: 'apps/web',
      port: 3000,
      env: { API_KEY: 'abc' },
      secret_env_keys: ['API_KEY'],
    })
    expect(sent?.confirm).toBe(false)
  })

  it('shows per-app problems from the dry run and creates nothing', async () => {
    const calls = mockFetch(() =>
      response({ dry_run: true }, [
        {
          package: 'web',
          name: 'mono-web',
          directory: 'apps/web',
          status: 'failed',
          code: 'invalid_port',
          message: 'set a port between 1 and 65535',
        },
        {
          package: 'api',
          name: 'mono-api',
          directory: 'apps/api',
          status: 'would_create',
        },
      ]),
    )
    mount()
    await userEvent.click(screen.getByRole('button', { name: /check 2 apps/i }))
    expect(
      await screen.findByText('set a port between 1 and 65535'),
    ).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(
      calls
        .filter((c) => c.url.includes('/apply'))
        .every((c) => c.body?.dry_run),
    ).toBe(true)
    expect(screen.getByRole('button', { name: 'Create 1 app' })).toBeEnabled()
  })

  it('asks for confirmation above the threshold before creating', async () => {
    const calls = mockFetch((b) =>
      b.dry_run
        ? response(
            { dry_run: true, needs_confirmation: true, confirm_above: 1 },
            [
              {
                package: 'web',
                name: 'mono-web',
                directory: 'apps/web',
                status: 'would_create',
              },
              {
                package: 'api',
                name: 'mono-api',
                directory: 'apps/api',
                status: 'would_create',
              },
            ],
          )
        : response({}, [
            {
              package: 'web',
              name: 'mono-web',
              directory: 'apps/web',
              status: 'created',
              app_id: 'a1',
            },
            {
              package: 'api',
              name: 'mono-api',
              directory: 'apps/api',
              status: 'created',
              app_id: 'a2',
            },
          ]),
    )
    mount()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check 2 apps/i }))
    await user.click(
      await screen.findByRole('button', { name: 'Create 2 apps' }),
    )
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/more than 1 apps/i)).toBeInTheDocument()
    expect(
      calls.filter((c) => !c.body?.dry_run && c.url.includes('apply')),
    ).toHaveLength(0)
    await user.click(
      within(dialog).getByRole('button', { name: 'Create 2 apps' }),
    )
    expect(
      await screen.findByRole('link', { name: 'mono-web' }),
    ).toHaveAttribute('href', '/apps/mono-web')
    const real = calls.find((c) => c.body && !c.body.dry_run)?.body
    expect(real?.confirm).toBe(true)
  })

  it('shows results and retries only the failed apps', async () => {
    let attempt = 0
    const calls = mockFetch((b) => {
      if (b.dry_run) {
        return response(
          { dry_run: true },
          b.packages.map((p) => ({
            package: p.package,
            name: p.name,
            directory: p.directory,
            status: 'would_create' as const,
          })),
        )
      }
      attempt++
      if (attempt === 1) {
        return response({}, [
          {
            package: 'web',
            name: 'mono-web',
            directory: 'apps/web',
            status: 'skipped_dependency',
            code: 'dependency_failed',
            message: 'skipped because mono-api was not created',
          },
          {
            package: 'api',
            name: 'mono-api',
            directory: 'apps/api',
            status: 'failed',
            code: 'git_source_failed',
            message: 'repository unreachable',
          },
        ])
      }
      return response(
        {},
        b.packages.map((p) => ({
          package: p.package,
          name: p.name,
          directory: p.directory,
          status: 'created' as const,
        })),
      )
    })
    mount()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /check 2 apps/i }))
    await user.click(
      await screen.findByRole('button', { name: 'Create 2 apps' }),
    )
    expect(
      await screen.findByText('repository unreachable'),
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Retry 2 failed apps' }),
    )
    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: /retry/i }),
      ).not.toBeInTheDocument()
    })
    const retry = calls.filter((c) => c.body && !c.body.dry_run).at(-1)?.body
    expect(retry?.packages.map((p) => p.directory).sort()).toEqual([
      'apps/api',
      'apps/web',
    ])
    expect(screen.getAllByText('Created')).toHaveLength(2)
  })
})
