import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import { CommonI18n } from '../test/commonI18n'
import type { DeployAttempt } from '../types/deployAttempt'
import { DeployPinnedStatus } from './DeployPinnedStatus'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, params }: { children: ReactNode; params: object }) => (
    <a href={`/logs/${(params as { deployId: string }).deployId}`}>
      {children}
    </a>
  ),
}))

const base: DeployAttempt = {
  id: 'dep_1',
  service_name: 'web',
  image: 'web:abc',
  status: 'failed',
  started_at: '2026-10-21T10:00:00Z',
  finished_at: '2026-10-21T10:01:05Z',
}

function renderStatus(attempt: DeployAttempt, stageLabel?: string) {
  render(
    <CommonI18n>
      <DeployPinnedStatus
        appName="web"
        attempt={attempt}
        stageLabel={stageLabel}
      />
    </CommonI18n>,
  )
}

describe('DeployPinnedStatus', () => {
  it('shows status, stage and duration with a log link when failed', () => {
    renderStatus(base, 'Build')
    expect(screen.getByTestId('deploy-pinned-status')).toHaveTextContent(
      /Failed · Build · /,
    )
    expect(
      screen.getByRole('link', { name: 'View failing stage log' }),
    ).toHaveAttribute('href', '/logs/dep_1')
  })

  it('has no failing log link for a successful attempt', () => {
    renderStatus({ ...base, status: 'succeeded' })
    expect(screen.queryByRole('link')).toBeNull()
  })
})
