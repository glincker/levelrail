import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NewAppI18n } from '../../../test/newAppI18n'
import { jsonResponse, nextAnalysis } from '../../../test/newAppFixtures'
import { applyNewApp, type DatabaseEdit } from '../../../lib/newApp'
import type { ImportAnalysis } from '../../../queries/importAnalyze'
import { DatabaseSection } from './DatabaseSection'
import { injectedVars, suggestDbName } from './databaseSectionLogic'

const withoutDb: ImportAnalysis = { ...nextAnalysis, databases: [], env: [] }

function stubApi(backupTargets: unknown[] = []) {
  const calls: { url: string; method: string }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({ url, method })
      if (url === '/api/v1/databases' && method === 'GET')
        return Promise.resolve(
          jsonResponse([{ name: 'main-pg', engine: 'postgres' }]),
        )
      if (url === '/api/v1/database-engines')
        return Promise.resolve(jsonResponse([]))
      if (url === '/api/v1/backup-targets')
        return Promise.resolve(jsonResponse(backupTargets))
      if (url.startsWith('/api/v1/database-versions'))
        return Promise.resolve(jsonResponse({ versions: [] }))
      return Promise.resolve(jsonResponse({ error: 'not found' }, 404))
    }),
  )
  return calls
}

function mount(analysis: ImportAnalysis, onChange = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <NewAppI18n>
        <DatabaseSection
          analysis={analysis}
          appName="shop"
          onChange={onChange}
        />
      </NewAppI18n>
    </QueryClientProvider>,
  )
  return onChange
}

function last(fn: ReturnType<typeof vi.fn>): DatabaseEdit[] | null {
  return fn.mock.calls.at(-1)?.[0] as DatabaseEdit[] | null
}

beforeEach(() => {
  stubApi()
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DatabaseSection', () => {
  it('starts collapsed and leaves the analysis defaults alone', () => {
    const onChange = mount(withoutDb)
    expect(screen.getByText('Needs a database?')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Postgres' })).toBeNull()
    expect(last(onChange)).toBeNull()
  })

  it('choosing an engine shows the summary, a free name and a masked variable', async () => {
    const user = userEvent.setup()
    const onChange = mount(withoutDb)
    await user.click(screen.getByText('Needs a database?'))
    await user.click(await screen.findByRole('button', { name: 'Postgres' }))
    expect(
      await screen.findByText(
        /Creates a Postgres 16 database and sets DATABASE_URL/,
      ),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Database name')).toHaveValue('shop-postgres')
    await waitFor(() => {
      expect(
        screen.getByText('This database name is available.'),
      ).toBeInTheDocument()
    })
    expect(screen.getByText('DATABASE_URL')).toBeInTheDocument()
    expect(screen.getByText('••••••••')).toBeInTheDocument()
    expect(last(onChange)?.[0]).toMatchObject({
      engine: 'postgres',
      name: 'shop-postgres',
      envVar: 'DATABASE_URL',
    })
  })

  it('attaches an existing database instead of creating one', async () => {
    const user = userEvent.setup()
    const onChange = mount(withoutDb)
    await user.click(screen.getByText('Needs a database?'))
    await user.click(
      await screen.findByRole('button', { name: 'Use an existing one' }),
    )
    await user.selectOptions(
      await screen.findByLabelText('Existing database'),
      'main-pg',
    )
    await waitFor(() => {
      expect(last(onChange)?.[0]).toMatchObject({
        name: 'main-pg',
        engine: 'postgres',
        version: '',
      })
    })
  })

  it('opens with the detected engine suggested', async () => {
    mount({
      ...withoutDb,
      databases: [
        {
          engine: 'redis',
          version: '7',
          suggested_name: 'shop-redis',
          env_var: 'REDIS_URL',
          field: 'url',
          evidence: 'ioredis',
        },
      ],
    })
    expect(
      await screen.findByRole('button', { name: 'Redis' }),
    ).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByLabelText('Database name')).toHaveValue('shop-redis')
    expect(screen.getByText('REDIS_URL')).toBeInTheDocument()
  })
})

describe('database env injection', () => {
  it('lists each variable once per database', () => {
    const base = { enabled: true, version: '', field: 'url' }
    expect(
      injectedVars([
        { ...base, engine: 'postgres', name: 'a', envVar: 'DATABASE_URL' },
        { ...base, engine: 'redis', name: 'b', envVar: 'REDIS_URL' },
        { ...base, engine: 'mysql', name: 'c', envVar: 'DATABASE_URL' },
      ]),
    ).toEqual([
      { key: 'DATABASE_URL', database: 'a' },
      { key: 'REDIS_URL', database: 'b' },
    ])
  })

  it('suggests a free name from the app name', () => {
    expect(suggestDbName('Shop', 'postgres', ['shop-postgres'])).toBe(
      'shop-postgres-2',
    )
  })
})

describe('failure handling', () => {
  it('does not create the app when the database cannot be created', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push(`${init?.method ?? 'GET'} ${url}`)
        if (url === '/api/v1/databases' && init?.method === 'POST')
          return Promise.resolve(jsonResponse({ error: 'no capacity' }, 500))
        return Promise.resolve(jsonResponse({ error: 'not found' }, 404))
      }),
    )
    const edit: DatabaseEdit = {
      enabled: true,
      engine: 'postgres',
      version: '16',
      name: 'shop-postgres',
      envVar: 'DATABASE_URL',
      field: 'url',
    }
    await expect(
      applyNewApp(
        withoutDb,
        { repoUrl: withoutDb.repo_url, branch: 'main' } as never,
        {
          name: 'shop',
          port: 3000,
          healthPath: '/',
          memoryMb: 512,
          buildType: 'railpack',
          dockerfilePath: '',
          baseDirectory: '',
          domain: '',
          env: {},
          databases: [edit],
        },
      ),
    ).rejects.toMatchObject({ step: 'database', appCreated: false })
    expect(calls.some((c) => c.startsWith('POST /api/v1/apps'))).toBe(false)
  })
})
