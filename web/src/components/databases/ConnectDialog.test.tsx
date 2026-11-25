import { cleanup, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../test/databasesI18n'
import type { DatabaseResource } from '../../types/databaseDetail'
import { ConnectDialog } from './ConnectDialog'

const net = vi.hoisted(() => ({ port: 27017, host: 'db-demo' }))

vi.mock('../../queries/databaseAccess', () => ({
  databaseAccessKeys: { network: (n: string) => ['n', n] },
  useDatabaseNetwork: () => ({
    data: {
      internal: { host: net.host, port: net.port },
      published: { host_port: net.port, bind: ['127.0.0.1'] },
      tls: { enabled: false, required: false },
    },
  }),
}))
vi.mock('../../queries/databases', () => ({
  useSetDatabasePublicAccess: () => ({ isPending: false, mutate: vi.fn() }),
}))
vi.mock('../env/StepUpDialog', () => ({ StepUpDialog: () => null }))

function show(database: Partial<DatabaseResource>) {
  render(
    <DatabasesI18n>
      <QueryClientProvider client={new QueryClient()}>
        <ConnectDialog
          database={database as DatabaseResource}
          open
          onOpenChange={() => undefined}
        />
      </QueryClientProvider>
    </DatabasesI18n>,
  )
}

describe('ConnectDialog', () => {
  afterEach(cleanup)

  it('mongodb shows the resource name as user and the admin auth source', () => {
    net.port = 27017
    show({ name: 'demo-mongodb', engine: 'mongodb', publicly_accessible: true })
    expect(screen.getAllByText('demo-mongodb').length).toBeGreaterThan(0)
    expect(
      screen.getAllByText(/authSource=admin&directConnection=true/).length,
    ).toBeGreaterThan(0)
    expect(screen.getByLabelText(/reveal password/i)).toBeTruthy()
  })

  it('valkey is passwordless with no reveal control', () => {
    net.port = 6379
    show({ name: 'demo-valkey', engine: 'valkey', publicly_accessible: true })
    expect(screen.queryByLabelText(/reveal password/i)).toBeNull()
    expect(
      screen.getAllByText(/redis:\/\/127\.0\.0\.1:6379/).length,
    ).toBeGreaterThan(0)
  })
})
