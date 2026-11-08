import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { BuildRow } from '../../queries/buildCenter'
import { BuildsBulkBar } from './BuildsBulkBar'
import { filterBuilds } from './BuildsFilterBar'

function row(id: string, status: string, over: Partial<BuildRow> = {}) {
  return {
    id,
    app: `app-${id}`,
    status,
    source: 'git',
    started_at: '2026-01-01T00:00:00Z',
    can_cancel: status === 'running',
    can_retry: status === 'failed',
    ...over,
  } as BuildRow
}

describe('filterBuilds', () => {
  it('filters by status and query', () => {
    const rows = [row('1', 'running'), row('2', 'failed'), row('3', 'held')]
    expect(filterBuilds(rows, { status: 'failed', query: '' })).toHaveLength(1)
    expect(filterBuilds(rows, { status: 'queued', query: '' })).toHaveLength(1)
    expect(filterBuilds(rows, { status: 'all', query: 'app-3' })).toHaveLength(
      1,
    )
  })
})

describe('BuildsBulkBar', () => {
  it('cancels once per selected cancelable row', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    const rows = [row('1', 'running'), row('2', 'running'), row('3', 'failed')]
    render(
      <QueryClientProvider client={new QueryClient()}>
        <BuildsBulkBar rows={rows} onClear={vi.fn()} />
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })
    expect(fetchMock.mock.calls[0]?.[0]).toContain('/builds/1/cancel')
  })
})
