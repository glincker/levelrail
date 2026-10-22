import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { DatabaseRow } from './DatabaseRow'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
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

function makeDb(over: Partial<DatabaseListEntry> = {}): DatabaseListEntry {
  return {
    name: 'orders-db',
    engine: 'postgres',
    version: '16',
    node_id: 'node-a',
    node_name: 'node-a',
    backup_schedule: '0 3 * * *',
    status: { label: 'Healthy', variant: 'success' },
    ...over,
  }
}

function renderRow(db: DatabaseListEntry) {
  return render(
    <DatabasesI18n>
      <DatabaseRow database={db} />
    </DatabasesI18n>,
  )
}

describe('DatabaseRow', () => {
  it('links the name and shows status, backups and node', () => {
    renderRow(makeDb())
    expect(screen.getByRole('link', { name: 'orders-db' })).toBeInTheDocument()
    expect(screen.getByText('Healthy')).toBeInTheDocument()
    expect(screen.getByText('Backups on')).toBeInTheDocument()
    expect(screen.getByText('node-a')).toBeInTheDocument()
  })

  it('shows this server and its hostname for a local placement', () => {
    renderRow(
      makeDb({ node_id: undefined, node_name: 'box-1', node_is_local: true }),
    )
    expect(screen.getByText('This server')).toBeInTheDocument()
    expect(screen.getByText('box-1')).toBeInTheDocument()
    expect(screen.queryByText('Unassigned')).not.toBeInTheDocument()
  })

  it('shows the resolved name for a remote node', () => {
    renderRow(makeDb({ node_id: 'n2', node_name: 'worker-2' }))
    expect(screen.getByText('worker-2')).toBeInTheDocument()
  })

  it('says so when backups are not scheduled and the node is gone', () => {
    renderRow(
      makeDb({
        backup_schedule: undefined,
        node_id: 'node-gone',
        node_name: undefined,
      }),
    )
    expect(screen.getByText('No backups')).toBeInTheDocument()
    expect(screen.getByText('Unassigned')).toBeInTheDocument()
  })

  it('marks an external database', () => {
    renderRow(
      makeDb({
        external: { host: 'db.example.com', port: 5432 },
      } as Partial<DatabaseListEntry>),
    )
    expect(screen.getAllByText('db.example.com:5432').length).toBeGreaterThan(0)
  })
})
