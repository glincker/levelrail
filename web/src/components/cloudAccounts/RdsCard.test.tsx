import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AwsRdsCluster, AwsRdsInstance } from '../../queries/cloudRds'
import { RdsCard } from './RdsCard'

const mutate = vi.fn()
const clusterMutate = vi.fn()
let instances: AwsRdsInstance[] = []
let clusters: AwsRdsCluster[] = []

vi.mock('../../queries/cloudRds', () => ({
  useAwsRds: () => ({ isPending: false, error: null, data: instances }),
  useAwsRdsAction: () => ({ mutate, isError: false, error: null }),
  useAwsRdsCreate: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
  useAwsRdsEstimate: () => ({ data: undefined }),
  useAwsRdsClusters: () => ({ data: { max_stop_days: 7, clusters } }),
  useAwsRdsClusterSnapshots: () => ({ data: [] }),
  useAwsRdsClusterAction: () => ({
    mutate: clusterMutate,
    isError: false,
    error: null,
  }),
  useAwsRdsClusterSnapshotDelete: () => ({ mutate: vi.fn() }),
}))

const base: AwsRdsInstance = {
  id: 'db1',
  engine: 'postgres',
  class: 'db.t4g.micro',
  status: 'available',
  host: 'h',
  port: 5432,
  allocated_gb: 20,
  multi_az: false,
  managed: true,
  est_monthly_usd: 12.5,
}

beforeEach(() => {
  mutate.mockReset()
  clusters = []
})

describe('RdsCard', () => {
  it('offers snapshot and stop for a managed running instance, never adopt', () => {
    instances = [base]
    render(<RdsCard id="acct" canCreate={false} />)
    expect(
      screen.getByRole('button', { name: 'services.rds.snapshot' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'services.rds.stop' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'services.rds.adopt' }),
    ).toBeNull()
    expect(
      screen.queryByRole('button', { name: 'services.rds.start' }),
    ).toBeNull()
  })

  it('offers only adopt for an unmanaged instance', async () => {
    instances = [{ ...base, managed: false }]
    render(<RdsCard id="acct" canCreate={false} />)
    expect(
      screen.queryByRole('button', { name: 'services.rds.stop' }),
    ).toBeNull()
    await userEvent.click(
      screen.getByRole('button', { name: 'services.rds.adopt' }),
    )
    expect(mutate).toHaveBeenCalledWith({ rid: 'db1', action: 'adopt' })
  })

  it('offers start for a stopped managed instance and hides create for non root', () => {
    instances = [{ ...base, status: 'stopped' }]
    render(<RdsCard id="acct" canCreate={false} />)
    expect(
      screen.getByRole('button', { name: 'services.rds.start' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'services.rds.create' }),
    ).toBeNull()
  })

  it('shows create for root', () => {
    instances = []
    render(<RdsCard id="acct" canCreate />)
    expect(
      screen.getByRole('button', { name: 'services.rds.create' }),
    ).toBeInTheDocument()
  })

  it('starts a stopped cluster and shows the auto restart note', async () => {
    instances = []
    clusters = [
      {
        id: 'c1',
        engine: 'aurora-postgresql',
        status: 'stopped',
        members: [],
        managed: true,
      },
    ]
    render(<RdsCard id="acct" canCreate={false} />)
    expect(screen.getByText(/services.rds.autoRestart/)).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'services.rds.start' }),
    )
    expect(clusterMutate).toHaveBeenCalledWith({ cid: 'c1', action: 'start' })
  })
})
