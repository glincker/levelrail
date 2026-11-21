import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AwsRdsInstance } from '../../queries/cloudRds'
import { RdsCard } from './RdsCard'

const mutate = vi.fn()
let instances: AwsRdsInstance[] = []

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
})
