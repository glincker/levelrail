import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RollbackCard } from './RollbackCard'
import type { DeployAttempt } from '../../types/deployAttempt'

const h = vi.hoisted(() => ({
  data: undefined as { serving?: string } | undefined,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('../../queries/deployControl', () => ({
  useRollbackToDeploy: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null,
    data: h.data,
  }),
}))

const attempts = [
  {
    id: 'dep_prev',
    status: 'succeeded',
    live: false,
    started_at: '2026-01-01T00:00:00Z',
  },
] as unknown as DeployAttempt[]

beforeEach(() => {
  h.data = undefined
})

describe('RollbackCard', () => {
  it('shows the going-live chip only while the rollback serving is pending', () => {
    const { unmount } = render(<RollbackCard name="site" attempts={attempts} />)
    expect(screen.queryByText('promote.pending')).toBeNull()
    unmount()
    h.data = { serving: 'pending' }
    render(<RollbackCard name="site" attempts={attempts} />)
    expect(screen.getByText('promote.pending')).toBeTruthy()
  })
})
