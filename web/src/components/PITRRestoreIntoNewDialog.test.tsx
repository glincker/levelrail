import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PointInTimeRestoreSection } from './PointInTimeRestoreSection'
import { DatabasesI18n } from '../test/databasesI18n'
import { createTestQueryClient } from '@/test/testQueryClient'

function fakeJsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function requestUrlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function renderSection(engine: string) {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <DatabasesI18n>
        <PointInTimeRestoreSection
          database={{ name: 'main', engine, version: '8.4' }}
        />
      </DatabasesI18n>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PointInTimeRestoreSection for MySQL and MariaDB', () => {
  it('offers binlog archiving instead of WAL when not enabled', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) => {
        const url = requestUrlOf(input)
        if (url.endsWith('/pitr')) {
          return Promise.resolve(
            fakeJsonResponse({ enabled: false, has_base_backup: false }),
          )
        }
        throw new Error(`unexpected fetch: ${url}`)
      }),
    )

    renderSection('mariadb')

    expect(await screen.findByText(/binary log archiving/i)).toBeInTheDocument()
    expect(screen.queryByText(/WAL archiving/i)).not.toBeInTheDocument()
  })

  it('restores into a new database and sends restore_as', async () => {
    let restoreBody: Record<string, string> = {}
    vi.stubGlobal(
      'fetch',
      vi.fn<
        (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
      >((input, init) => {
        const url = requestUrlOf(input)
        if (url.endsWith('/pitr-restore') && init?.method === 'POST') {
          restoreBody = JSON.parse(
            typeof init.body === 'string' ? init.body : '{}',
          ) as Record<string, string>
          return Promise.resolve(
            fakeJsonResponse(
              {
                id: 'pitr_1',
                database_name: 'main',
                base_backup_history_id: 'bb_1',
                target_timestamp: restoreBody.target_time,
                status: 'running',
                started_at: '2026-08-14T05:00:00Z',
              },
              202,
            ),
          )
        }
        if (url.endsWith('/pitr')) {
          return Promise.resolve(
            fakeJsonResponse({
              enabled: true,
              enabled_at: '2026-08-14T00:00:00Z',
              has_base_backup: true,
              window_start: '2026-08-14T00:00:00Z',
              window_end: '2026-08-14T06:00:00Z',
              restore_mode: 'new_database',
            }),
          )
        }
        if (url.endsWith('/base-backups')) {
          return Promise.resolve(
            fakeJsonResponse([
              {
                id: 'bb_1',
                database_name: 'main',
                target_id: 't',
                size_bytes: 1,
                status: 'succeeded',
                started_at: '2026-08-14T01:00:00Z',
              },
            ]),
          )
        }
        if (url.endsWith('/pitr-restores')) {
          return Promise.resolve(fakeJsonResponse([]))
        }
        if (url.includes('/backup-targets')) {
          return Promise.resolve(
            fakeJsonResponse([
              {
                id: 'bkt_1',
                name: 'primary',
                provider: 'aws',
                bucket: 'backups',
              },
            ]),
          )
        }
        throw new Error(`unexpected fetch: ${url}`)
      }),
    )

    const user = userEvent.setup()
    renderSection('mysql')

    const open = await screen.findByRole('button', {
      name: 'Restore to timestamp',
    })
    await waitFor(() => {
      expect(open).toBeEnabled()
    })
    await user.click(open)

    const when = await screen.findByLabelText('Restore to')
    await user.type(when, '2026-08-14T03:00:00')
    const name = screen.getByLabelText('New database name')
    await user.clear(name)
    await user.type(name, 'main-copy')
    await user.click(
      screen.getByRole('button', { name: 'Restore into new database' }),
    )

    await waitFor(() => {
      expect(restoreBody.restore_as).toBe('main-copy')
    })
    expect(restoreBody.base_backup_id).toBe('bb_1')
    expect(
      screen.queryByLabelText(/Type .* to confirm/),
    ).not.toBeInTheDocument()
  })
})
