import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../test/databasesI18n'
import { DatabasesTable } from '../DatabasesTable'
import { AppCard } from './AppCard'
import { AppsTable } from './AppsTable'
import type { AppListEntry } from '../../types/appDetail'
import type { DatabaseListEntry } from '../../types/databaseDetail'

// The tag's own gating is covered in stacks/ManagedByStackNotice.test.tsx.
vi.mock('../stacks/ManagedByStackNotice', () => ({
  ManagedByStackTag: ({ stack }: { stack?: string }) =>
    stack ? <span>{`tag:${stack}`}</span> : null,
}))
vi.mock('../AppRowActions', () => ({ AppRowActions: () => null }))
vi.mock('./AppQuickActions', () => ({ AppQuickActions: () => null }))
vi.mock('./AppMetricCells', () => ({
  ErrorCell: () => null,
  LastDeployCell: () => null,
  P95Cell: () => null,
  TrafficSpark: () => null,
}))
vi.mock('./useAppRowMetrics', () => ({ useAppRowMetrics: () => undefined }))
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

const app: AppListEntry = {
  name: 'web-a',
  image: 'nginx:1.27',
  port: 80,
  domains: [],
  environment_name: 'production',
  status: { label: 'Healthy', variant: 'success' },
  bind_address: '',
  strategy: 'recreate',
  replicas: 1,
  suspended: false,
  env_dirty: false,
  managed_by_stack: 'web-stack',
}

function wrap(ui: ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <DatabasesI18n>{ui}</DatabasesI18n>
    </QueryClientProvider>,
  )
}

describe('managed-by tag in list renderings', () => {
  it('shows it in the apps table view', () => {
    wrap(<AppsTable apps={[app]} selected={[]} onSelect={vi.fn()} />)
    expect(screen.getAllByText('tag:web-stack').length).toBeGreaterThan(0)
  })

  it('shows it in the apps grid card', () => {
    wrap(<AppCard app={app} selected={false} onSelect={vi.fn()} />)
    expect(screen.getByText('tag:web-stack')).toBeInTheDocument()
  })

  it('shows it in the databases table', () => {
    const db: DatabaseListEntry = {
      name: 'orders-db',
      engine: 'postgres',
      version: '16',
      status: { label: 'Healthy', variant: 'success' },
      managed_by_stack: 'db-stack',
    }
    wrap(<DatabasesTable databases={[db]} />)
    expect(screen.getAllByText('tag:db-stack').length).toBeGreaterThan(0)
  })
})
