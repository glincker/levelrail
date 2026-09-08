import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DeleteBackupHistoryDialog } from './DeleteBackupHistoryDialog'
import type { BackupHistoryRecord } from '../types/backupHistory'

function requestUrlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

const backup: BackupHistoryRecord = {
  id: 'bkh_1',
  database_name: 'main',
  target_id: 'bkt_1',
  object_key: 'main/main-1.dump',
  size_bytes: 4096,
  status: 'succeeded',
  started_at: '2026-08-14T00:00:00Z',
  finished_at: '2026-08-14T00:01:00Z',
}

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <DeleteBackupHistoryDialog databaseName="main" backup={backup} />
    </QueryClientProvider>,
  )
}

describe('DeleteBackupHistoryDialog', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  let deleteStatus: number

  beforeEach(() => {
    deleteStatus = 204
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestUrlOf(input)
      const method = init?.method ?? 'GET'
      if (url === '/api/v1/databases/main/backups/bkh_1' && method === 'DELETE') {
        if (deleteStatus === 204) {
          return Promise.resolve({ ok: true, status: 204 } as unknown as Response)
        }
        return Promise.resolve({
          ok: false,
          status: deleteStatus,
          json: () => Promise.resolve({ error: 'backup not found' }),
        } as unknown as Response)
      }
      return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('does not call the API until the confirm button is clicked', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Delete this backup' }))
    await screen.findByRole('dialog')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('deletes the backup on confirm', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Delete this backup' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([input, init]) =>
            requestUrlOf(input as RequestInfo) === '/api/v1/databases/main/backups/bkh_1' &&
            (init as RequestInit)?.method === 'DELETE',
        ),
      ).toBe(true)
    })

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('shows the server error and keeps the dialog open on failure', async () => {
    deleteStatus = 404
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Delete this backup' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await screen.findByText('backup not found')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('closes without deleting when cancel is clicked', async () => {
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Delete this backup' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
