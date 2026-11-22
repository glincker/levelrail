import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AwsSnapshot } from '../../queries/cloudAwsOps'
import { IdleCard } from './IdleCard'
import { SnapshotsCard } from './SnapshotsCard'

const del = vi.fn()
const create = vi.fn()
let snaps: AwsSnapshot[] = []

vi.mock('../../queries/cloudAwsOps', () => ({
  useAwsSnapshots: () => ({ isPending: false, error: null, data: snaps }),
  useAwsVolumes: () => ({ isPending: false, error: null, data: [] }),
  useAwsSnapshotCreate: () => ({
    mutate: create,
    isPending: false,
    error: null,
  }),
  useAwsSnapshotDelete: () => ({ mutate: del, isPending: false, error: null }),
  useAwsIdle: () => ({
    isPending: false,
    error: null,
    data: [
      {
        kind: 'ebs_unattached',
        id: 'vol-1',
        detail: 'd',
        action: 'snapshot_delete',
        est_monthly_usd: 1.5,
      },
      {
        kind: 'eip_unassociated',
        id: 'eip-1',
        detail: 'd',
        action: 'release',
        est_monthly_usd: 3.6,
      },
    ],
  }),
}))

const snap: AwsSnapshot = {
  id: 'snap-1',
  volume_id: 'vol-1',
  state: 'completed',
  size_gb: 8,
  started_at: '',
  description: '',
  managed: true,
  expires_at: '',
}

beforeEach(() => {
  del.mockReset()
  create.mockReset()
})

describe('SnapshotsCard', () => {
  it('shows delete only for managed snapshots', async () => {
    snaps = [snap, { ...snap, id: 'snap-2', managed: false }]
    render(<SnapshotsCard id="acct" />)
    const buttons = screen.getAllByRole('button', {
      name: 'services.ebs.delete',
    })
    expect(buttons).toHaveLength(1)
    await userEvent.click(buttons[0] as HTMLElement)
    expect(del).toHaveBeenCalledWith('snap-1')
  })

  it('creates from a volume id', async () => {
    snaps = []
    render(<SnapshotsCard id="acct" />)
    await userEvent.type(
      screen.getByRole('textbox', { name: 'services.ebs.target' }),
      'vol-9',
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'services.ebs.create' }),
    )
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ volume_id: 'vol-9' }),
      expect.anything(),
    )
  })
})

describe('IdleCard', () => {
  it('states dry run and totals the saving', () => {
    render(<IdleCard id="acct" />)
    expect(screen.getByText('services.cloudwatch.idleNote')).toBeInTheDocument()
    expect(
      screen.getByText('services.cloudwatch.idleTotal'),
    ).toBeInTheDocument()
  })
})
