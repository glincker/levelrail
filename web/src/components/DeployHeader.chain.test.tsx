import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DeployDetailI18n } from '../test/deployDetailI18n'
import type { DeployAttempt } from '../types/deployAttempt'
import { DeployHeader } from './DeployHeader'

// No <RouterProvider> is mounted, so Link and useNavigate are stubbed.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
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
vi.mock('../queries/deployControl', () => ({
  useRollbackToDeploy: () => ({ isPending: false, mutate: vi.fn() }),
}))

const attempt: DeployAttempt = {
  id: 'dep_3',
  service_name: 'web',
  image: 'web:3',
  status: 'failed',
  started_at: '2026-09-24T10:00:00Z',
}

function renderHeader(a: DeployAttempt) {
  render(
    <DeployDetailI18n>
      <DeployHeader appName="web" attempt={a} isLive={false} />
    </DeployDetailI18n>,
  )
}

describe('DeployHeader chain position', () => {
  it('names the attempt number when the deploy is part of a chain', () => {
    renderHeader({ ...attempt, rerun_of: 'dep_2', chain_position: 3 })
    expect(screen.getByText('Attempt 3 of this chain')).toBeTruthy()
  })

  it.each([undefined, 1])('stays quiet for chain position %s', (position) => {
    renderHeader({ ...attempt, chain_position: position })
    expect(screen.queryByText(/of this chain/)).toBeNull()
  })
})
