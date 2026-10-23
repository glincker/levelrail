import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { MigrateAgentPanel } from './MigrateAgentPanel'

const session = {
  id: 'ms_1',
  label: 'old box',
  state: 'connected',
  token_expires_at: '2026-10-21T10:00:00Z',
  session_expires_at: '2026-10-24T10:00:00Z',
  created_at: '2026-10-21T09:00:00Z',
}

const detail = {
  session,
  inventory: {
    docker_version: '27',
    volumes: [{ name: 'data', driver: 'local' }],
    network_count: 1,
    containers: [
      {
        id: 'a',
        name: 'shop-web-1',
        image: 'web',
        state: 'running',
        unlisted: false,
      },
      {
        id: 'b',
        name: 'stray-cron',
        image: 'alpine',
        state: 'running',
        unlisted: true,
      },
      {
        id: 'c',
        name: 'pg',
        image: 'postgres:16',
        state: 'running',
        unlisted: false,
        database_engine: 'postgres',
        database_version: '16',
      },
    ],
  },
}

const verifyBody = {
  comparisons: [
    {
      name: 'app-shop-uploads',
      source: 'uploads',
      source_files: 3,
      source_bytes: 90,
      target_files: 3,
      target_bytes: 90,
      status: 'match',
    },
  ],
  advance: { advanced: true },
  databases_on_source: 1,
  checklist: { app: 'shop' },
}

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

describe('MigrateAgentPanel', () => {
  const calls: string[] = []
  beforeEach(() => {
    calls.length = 0
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push(`${init?.method ?? 'GET'} ${url}`)
        if (
          url === '/api/v1/migration/agent/sessions' &&
          init?.method === 'POST'
        ) {
          return json(
            {
              id: 'ms_2',
              token: 'mig_ms_2.secret',
              expires_at: '2026-10-21T10:00:00Z',
              ca_fingerprint: 'abcd',
              binary: 'x-migrate',
              command:
                'sudo APP_MIGRATION_TOKEN=mig_ms_2.secret x-migrate install',
              session_max_hours: 72,
            },
            201,
          )
        }
        if (url === '/api/v1/migration/agent/sessions')
          return json({ sessions: [session] })
        if (url === '/api/v1/migration/agent/sessions/ms_1') return json(detail)
        if (url.endsWith('/verify')) return json(verifyBody)
        if (url === '/api/v1/migration/checklists') {
          return json({
            checklists: [
              {
                app: 'shop',
                steps: [],
                current: 'data_verified',
                progress: 1,
                total: 5,
                tracked: true,
                volumes: [{ name: 'app-shop-uploads', target_command: 'x' }],
              },
            ],
          })
        }
        return json({ error: 'unexpected ' + url }, 500)
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function renderPanel() {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={qc}>
        <AllI18n>
          <MigrateAgentPanel />
        </AllI18n>
      </QueryClientProvider>,
    )
  }

  it('shows the connected source, flags unlisted containers and databases', async () => {
    renderPanel()
    expect(await screen.findByText('stray-cron')).toBeInTheDocument()
    expect(
      screen.getByText('Not matched to an imported app'),
    ).toBeInTheDocument()
    expect(screen.getByText('postgres 16')).toBeInTheDocument()
    expect(screen.getByText('Connected')).toBeInTheDocument()
  })

  it('creates a token and shows the one-line command once', async () => {
    const user = userEvent.setup()
    renderPanel()
    await user.click(
      await screen.findByRole('button', { name: 'Create install command' }),
    )
    expect(
      await screen.findByText(/APP_MIGRATION_TOKEN=mig_ms_2\.secret/),
    ).toBeInTheDocument()
  })

  it('verifies an app and offers an undo when the data step advanced', async () => {
    const user = userEvent.setup()
    renderPanel()
    await user.click(
      await screen.findByRole('button', { name: 'Verify volumes' }),
    )
    await waitFor(() =>
      expect(screen.getByText('Data step completed')).toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: 'Undo' })).toBeInTheDocument()
    expect(screen.getByText('Databases are still manual')).toBeInTheDocument()
    expect(
      calls.some((c) => c.startsWith('POST') && c.endsWith('/verify')),
    ).toBe(true)
  })
})
