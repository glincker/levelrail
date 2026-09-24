import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import type { DeployAttempt } from '../types/deployAttempt'
import type { DeployStage } from '../lib/deployStages'
import { DeployFailureSummaryCard } from './DeployFailureSummaryCard'

const redeploy = vi.fn()
vi.mock('../hooks/useRedeployApp', () => ({
  useRedeployApp: () => ({ isPending: false, redeploy }),
}))
const mutate = vi.fn()
vi.mock('../queries/deploys', () => ({
  useTriggerDeploy: () => ({ isPending: false, mutate }),
}))

const failed: DeployAttempt = {
  id: 'f',
  service_name: 'web',
  image: 'web:2',
  status: 'failed',
  started_at: '2026-09-24T10:00:00Z',
  error: `failed to read dockerfile: open Dockerfile: no such file ${'x'.repeat(300)}`,
}
const good: DeployAttempt = {
  ...failed,
  id: 'g',
  image: 'web:1',
  status: 'succeeded',
  started_at: '2026-09-24T09:00:00Z',
  error: undefined,
}
const stages: DeployStage[] = [
  { key: 'build', label: 'Build', status: 'failed', detail: failed.error },
  { key: 'rollout', label: 'Roll out', status: 'skipped' },
]

function renderCard(attempts: DeployAttempt[]) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <DeployFailureSummaryCard
        appName="web"
        attempt={failed}
        attempts={attempts}
        stages={stages}
        conditions={[]}
        lines={[]}
      />
    </QueryClientProvider>,
  )
}

describe('DeployFailureSummaryCard', () => {
  it('shows stage, cause, expandable error and actions', () => {
    renderCard([failed, good])
    expect(
      screen.getByRole('heading', { name: 'What went wrong' }),
    ).toBeTruthy()
    expect(
      screen.getByText('Dockerfile or build context not found'),
    ).toBeTruthy()
    const toggle = screen.getByRole('button', { name: 'Show more' })
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    fireEvent.click(toggle)
    expect(screen.getByRole('button', { name: 'Show less' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /Retry deploy/ }))
    expect(redeploy).toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: /Roll back to last good/ }),
    )
    expect(mutate).toHaveBeenCalledWith({ image: 'web:1' }, expect.any(Object))
  })

  it('hides rollback when there is no last good attempt', () => {
    renderCard([failed])
    expect(screen.queryByRole('button', { name: /Roll back/ })).toBeNull()
  })
})
