import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { planBulk } from '../lib/databasesFilter'
import { toItemResult } from '../queries/databasesBulk'
import { DatabasesBulkBar } from './DatabasesBulkBar'

function db(over: Partial<DatabaseListEntry>): DatabaseListEntry {
  return {
    name: 'db',
    engine: 'postgres',
    version: '16',
    status: { label: 'Healthy', variant: 'success' },
    ...over,
  }
}

function renderBar(selected: DatabaseListEntry[]) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <DatabasesI18n>
        <DatabasesBulkBar selected={selected} onClear={vi.fn()} />
      </DatabasesI18n>
    </QueryClientProvider>,
  )
}

describe('DatabasesBulkBar', () => {
  it('shows the selected count in a live region and the actions', () => {
    renderBar([db({ name: 'a' }), db({ name: 'b' })])
    expect(screen.getByRole('status')).toHaveTextContent('2 selected')
    const bar = screen.getByRole('region', { name: 'Bulk actions' })
    expect(bar).toHaveTextContent('Start')
    expect(bar).toHaveTextContent('Stop')
    expect(bar).toHaveTextContent('Back up')
  })

  it('renders no bar when nothing is selected', () => {
    renderBar([])
    expect(screen.queryByRole('region')).toBeNull()
  })

  it('plans skips before any call', () => {
    const plan = planBulk('backup', [
      db({ name: 'a', backup_target_id: 't1' }),
      db({ name: 'b' }),
      db({ name: 'c', suspended: true, backup_target_id: 't1' }),
    ])
    expect(plan.map((p) => p.status)).toEqual(['planned', 'skipped', 'planned'])
    expect(planBulk('start', [db({ name: 'a' })])[0]?.reason).toBe('running')
  })

  it('maps server statuses and keeps skip reason codes', () => {
    expect(
      toItemResult({ name: 'a', status: 'not_found', message: 'gone' }),
    ).toEqual({ name: 'a', status: 'notFound', reason: 'gone' })
    expect(
      toItemResult({
        name: 'b',
        status: 'skipped',
        reason: 'noBackups',
        message: 'x',
      }).reason,
    ).toBe('noBackups')
  })

  it('sends one bulk call with the typed count for stop', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            results: [
              { name: 'a', status: 'ok' },
              {
                name: 'b',
                status: 'protected',
                message: 'in protected environment',
              },
            ],
          }),
          { status: 207, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderBar([
      db({ name: 'a' }),
      db({ name: 'b' }),
      db({ name: 'c', suspended: true }),
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Stop' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Stop 2' }))
    await waitFor(() => {
      expect(screen.getByText('Protected')).toBeInTheDocument()
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ]
    expect(url).toBe('/api/v1/databases/bulk')
    expect(JSON.parse(init.body as string)).toEqual({
      action: 'stop',
      names: ['a', 'b'],
      confirm_count: 2,
    })
    vi.unstubAllGlobals()
  })
})
