import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../../test/databasesI18n'
import type { DatabaseResource } from '../../../types/databaseDetail'
import { ConnectionCard } from './ConnectionCard'

const net = vi.hoisted(() => ({
  mesh: undefined as { host: string; port: number } | undefined,
}))

vi.mock('../../../queries/databaseAccess', () => ({
  useDatabaseNetwork: () => ({
    isPending: false,
    refetch: vi.fn(),
    data: {
      internal: { host: 'db-m1-db', port: 5432 },
      mesh: net.mesh,
      tls: { enabled: false },
    },
  }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

const database = { name: 'm1-db', engine: 'postgres' } as DatabaseResource

describe('ConnectionCard', () => {
  afterEach(cleanup)

  it('shows the mesh address apps get for a remote database', () => {
    net.mesh = { host: 'm1-db.levelrail', port: 21000 }
    render(
      <DatabasesI18n>
        <ConnectionCard database={database} />
      </DatabasesI18n>,
    )
    expect(screen.getByText('For apps (over the mesh)')).toBeInTheDocument()
    expect(
      screen.getAllByText(/m1-db\.levelrail:21000/).length,
    ).toBeGreaterThan(0)
  })

  it('has no mesh section for a local database', () => {
    net.mesh = undefined
    render(
      <DatabasesI18n>
        <ConnectionCard database={database} />
      </DatabasesI18n>,
    )
    expect(screen.queryByText('For apps (over the mesh)')).toBeNull()
  })
})
