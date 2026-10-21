import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../test/commonI18n'
import type { DeployAttempt } from '../types/deployAttempt'
import { DeployAttemptRow } from './DeployAttemptRow'

// No <RouterProvider> is mounted, so Link and useNavigate are stubbed.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    Link: ({
      children,
      to,
      params,
      ...rest
    }: {
      children?: ReactNode
      to?: string
      params?: Record<string, string>
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a
        href={(to ?? '').replace(
          /\$(\w+)/g,
          (_m, key: string) => params?.[key] ?? '',
        )}
        {...rest}
      >
        {children}
      </a>
    ),
  }
})
vi.mock('../queries/deploys', () => ({
  isPendingApproval: () => false,
  useTriggerDeploy: () => ({ isPending: false, mutate: vi.fn() }),
}))
vi.mock('../queries/deployControl', () => ({
  useRollbackToDeploy: () => ({ isPending: false, mutate: vi.fn() }),
}))
vi.mock('../queries/publishLock', () => ({
  usePublishLockActions: () => ({
    publish: { mutate: vi.fn() },
    rollbackAndLock: { isPending: false, mutate: vi.fn() },
  }),
}))

const base: DeployAttempt = {
  id: 'dep_second',
  service_name: 'web',
  image: 'web:2',
  status: 'failed',
  started_at: '2026-09-24T10:00:00Z',
}

function renderRow(attempt: DeployAttempt) {
  render(
    <CommonI18n>
      <DeployAttemptRow
        appName="web"
        attempt={attempt}
        isLatestAttempt={false}
        conditions={[]}
        isSelected={false}
        onToggleSelected={vi.fn()}
        protectedEnv={undefined}
        ackProtected={false}
      />
    </CommonI18n>,
  )
}

describe('DeployAttemptRow rerun lineage', () => {
  it('links a rerun to the logs of the attempt it re-ran', () => {
    renderRow({ ...base, rerun_of: 'dep_firstattempt1234' })
    const link = screen.getByRole('link', { name: /dep_firstattempt1234/ })
    expect(link.textContent).toBe('Retry of dep_firstatt')
    expect(link.getAttribute('href')).toBe(
      '/apps/web/deploys/dep_firstattempt1234/logs',
    )
  })

  it('shows no badge for an attempt that is not a rerun', () => {
    renderRow(base)
    expect(screen.queryByText(/Retry of/)).toBeNull()
  })
})
