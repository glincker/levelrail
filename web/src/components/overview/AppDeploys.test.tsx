import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { DeployAttempt } from '../../types/deployAttempt'
import { AppDeploys } from './AppDeploys'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
  useNavigate: () => vi.fn(),
}))
vi.mock('@/components/kit', () => ({
  RelativeTime: () => <span>ago</span>,
}))

function attempt(i: number, over: Partial<DeployAttempt> = {}): DeployAttempt {
  return {
    id: `d${i}`,
    service_name: 'web',
    image: 'img',
    status: 'succeeded',
    started_at: '2026-01-01T00:00:00Z',
    finished_at: '2026-01-01T00:01:05Z',
    commit_message: `msg ${i}\nbody`,
    commit_sha: `abcdef${i}0000`,
    ...over,
  }
}

describe('AppDeploys', () => {
  it('shows an empty line without attempts', () => {
    render(<AppDeploys name="web" attempts={[]} />)
    expect(screen.getByText('deploys.none')).toBeInTheDocument()
  })

  it('caps at five rows and offers rollback only on older successes', () => {
    const rows = Array.from({ length: 7 }, (_, i) =>
      attempt(i, i === 0 ? { live: true } : {}),
    )
    render(<AppDeploys name="web" attempts={rows} />)
    expect(screen.getAllByTestId('deploy-row')).toHaveLength(5)
    expect(screen.getByText('msg 0')).toBeInTheDocument()
    expect(screen.getAllByText('deploys.rollback')).toHaveLength(4)
    expect(screen.getAllByText('duration')).toHaveLength(5)
  })
})
