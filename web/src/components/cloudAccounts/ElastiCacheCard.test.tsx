import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AwsCacheGroup } from '../../queries/cloudElastiCache'
import { ElastiCacheCard } from './ElastiCacheCard'

const mutate = vi.fn()
let groups: AwsCacheGroup[] = []

vi.mock('../../queries/cloudElastiCache', () => ({
  useAwsCacheGroups: () => ({ isPending: false, error: null, data: groups }),
  useAwsCacheClusters: () => ({ isPending: false, error: null, data: [] }),
  useAwsCacheAction: () => ({
    mutate,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

const base: AwsCacheGroup = {
  id: 'rg1',
  status: 'available',
  node_type: 'cache.t4g.micro',
  primary_endpoint: 'h',
  port: 6379,
  members: [],
  managed: true,
}

beforeEach(() => {
  mutate.mockReset()
})

describe('ElastiCacheCard', () => {
  it('offers snapshot, and delete only for root, on a managed group', async () => {
    groups = [base]
    const { rerender } = render(<ElastiCacheCard id="a" canDelete={false} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'services.elasticache.snapshot' }),
    )
    expect(mutate).toHaveBeenCalledWith({ kind: 'snapshot', id: 'rg1' })
    expect(
      screen.queryByRole('button', { name: 'services.elasticache.delete' }),
    ).toBeNull()
    rerender(<ElastiCacheCard id="a" canDelete />)
    expect(
      screen.getByRole('button', { name: 'services.elasticache.delete' }),
    ).toBeInTheDocument()
  })

  it('offers no change on an unmanaged group', () => {
    groups = [{ ...base, managed: false }]
    render(<ElastiCacheCard id="a" canDelete />)
    expect(
      screen.queryByRole('button', { name: 'services.elasticache.snapshot' }),
    ).toBeNull()
  })
})
