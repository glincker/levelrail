import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import databasesEn from '../../locales/en/databases.json'
import { classifyExplorerError } from '../../lib/explorerError'
import { ApiError } from '../../lib/apiError'
import { SchemaExplorer } from './SchemaExplorer'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['databases'],
  defaultNS: 'databases',
  resources: { en: { databases: databasesEn } },
  interpolation: { escapeValue: false },
})

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const LIMITS = { max_rows: 1000, max_cell_bytes: 65536, timeout_ms: 15000 }

function table(name: string) {
  return {
    name,
    kind: 'table',
    row_estimate: 10,
    size_bytes: 8192,
    columns: [],
    indexes: [],
  }
}

function schemaBody(over: Record<string, unknown>) {
  return {
    engine: 'postgres',
    schemas: [],
    table_limit: 5000,
    truncated: false,
    limits: LIMITS,
    checked_at: '2026-10-09T12:00:00Z',
    ...over,
  }
}

function stubFetch(handlers: {
  schema: () => Promise<Response>
  copy?: unknown
}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (String(url).includes('/schema')) return handlers.schema()
      if (String(url).includes('/imports/platform/')) {
        const ok = handlers.copy !== undefined
        return Promise.resolve({
          ok,
          status: ok ? 200 : 404,
          json: () => Promise.resolve(handlers.copy ?? {}),
        } as unknown as Response)
      }
      return Promise.resolve({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ error: 'nf' }),
      } as unknown as Response)
    }),
  )
}

function json(status: number, body: unknown): Promise<Response> {
  return Promise.resolve({
    ok: status < 400,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response)
}

function renderExplorer() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <SchemaExplorer databaseName="main" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('explorer states', () => {
  it('shows a loading state while the schema is read', () => {
    stubFetch({ schema: () => new Promise<Response>(() => undefined) })
    renderExplorer()
    expect(screen.getByRole('status').textContent).toContain('Reading')
  })

  it('shows the reason and raw error, never empty, when the read fails', async () => {
    stubFetch({
      schema: () =>
        json(400, {
          error: 'ERROR: permission denied for schema public',
        }),
    })
    renderExplorer()
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Could not read this database')
    expect(alert.textContent).toContain('permission denied')
    expect(alert.textContent).toContain('Retry')
    expect(screen.queryByText('No tables yet')).toBeNull()
    expect(
      screen.getByText('ERROR: permission denied for schema public'),
    ).toBeTruthy()
  })

  it('shows evidence for a truly empty database', async () => {
    stubFetch({
      schema: () =>
        json(
          200,
          schemaBody({
            evidence: {
              database_size_bytes: 8_000_000,
              schemas_scanned: 1,
              user_tables: 0,
              excluded_schemas: ['pg_catalog', 'information_schema'],
            },
          }),
        ),
    })
    renderExplorer()
    expect(await screen.findByText('No tables yet')).toBeTruthy()
    expect(screen.getByText('Database size on disk')).toBeTruthy()
    expect(screen.getByText(/pg_catalog, information_schema/)).toBeTruthy()
    expect(screen.getByText('Open the SQL console')).toBeTruthy()
  })

  it('flags a verified copy that is empty as an error', async () => {
    stubFetch({
      schema: () =>
        json(
          200,
          schemaBody({
            evidence: {
              database_size_bytes: 8_000_000,
              schemas_scanned: 1,
              user_tables: 0,
              excluded_schemas: ['pg_catalog'],
            },
          }),
        ),
      copy: { database: 'main', status: 'verified', checked: 0, mismatched: 0 },
    })
    renderExplorer()
    expect(
      await screen.findByText(/created as a migration target/),
    ).toBeTruthy()
    expect(screen.getByText('verified')).toBeTruthy()
    expect(screen.getByText(/should never be empty/)).toBeTruthy()
    expect(screen.queryByText('Open the SQL console')).toBeNull()
  })

  it('distinguishes a search that matched nothing from an empty database', async () => {
    stubFetch({
      schema: () =>
        json(
          200,
          schemaBody({
            schemas: [{ name: 'public', tables: [table('users')] }],
          }),
        ),
    })
    renderExplorer()
    const search = await screen.findByLabelText('Search tables')
    fireEvent.change(search, { target: { value: 'zzz' } })
    await waitFor(() => {
      expect(screen.getByText(/No table matches/)).toBeTruthy()
    })
    expect(screen.getByText(/Only your search filtered them out/)).toBeTruthy()
    expect(screen.queryByText('No tables yet')).toBeNull()
  })
})

describe('classifyExplorerError', () => {
  it('maps messages and statuses to plain reasons', () => {
    const c = (status: number, msg: string) =>
      classifyExplorerError(new ApiError(status, msg)).reason
    expect(c(400, 'FATAL: permission denied')).toBe('permission')
    expect(c(400, 'connection refused')).toBe('connection')
    expect(c(408, 'query timed out')).toBe('timeout')
    expect(c(500, 'internal error')).toBe('helper')
    expect(c(501, 'exec is not configured')).toBe('unsupported')
    expect(classifyExplorerError(new TypeError('Failed to fetch')).reason).toBe(
      'network',
    )
  })
})
