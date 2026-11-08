import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { planBulk } from '../lib/databasesFilter'
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
})
