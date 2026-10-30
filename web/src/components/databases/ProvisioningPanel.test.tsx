import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProvisioningView } from './ProvisioningPanel'
import type { ProvisionAttempt } from '../../queries/databaseProvision'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/x">{children}</a>
  ),
}))
vi.mock('@tanstack/react-query', () => ({ useQueryClient: () => ({}) }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, string>) =>
      o?.step ? `${k} ${o.step}` : k,
  }),
}))

const STEPS = [
  'accepted',
  'docker_check',
  'pull_image',
  'create_volume',
  'credentials_and_tls',
  'create_container',
  'start',
  'wait_ready',
  'apply_settings',
  'ready',
]

function attempt(
  status: ProvisionAttempt['status'],
  over: Record<string, Partial<ProvisionAttempt['steps'][number]>>,
  failure?: ProvisionAttempt['failure'],
): ProvisionAttempt {
  return {
    attempt_id: 'prov_1',
    status,
    started_at: '2026-10-21T10:00:00Z',
    steps: STEPS.map((step) => ({ step, status: 'pending', ...over[step] })),
    events: [],
    failure,
  }
}

describe('ProvisioningView', () => {
  afterEach(cleanup)

  it('shows pending steps', () => {
    render(
      <ProvisioningView
        name="main"
        attempt={attempt('in_progress', { accepted: { status: 'done' } })}
        onFix={vi.fn()}
      />,
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(STEPS.length)
    expect(screen.getByText('provision.title')).toBeTruthy()
  })

  it('shows the image pull percent', () => {
    render(
      <ProvisioningView
        name="main"
        attempt={attempt('in_progress', {
          pull_image: {
            status: 'started',
            current: 42,
            total: 100,
            detail: '2 of 5 layers',
          },
        })}
        onFix={vi.fn()}
      />,
    )
    expect(screen.getByText(/42%/)).toBeTruthy()
  })

  it('shows a failure and runs its fix', async () => {
    const onFix = vi.fn()
    render(
      <ProvisioningView
        name="main"
        attempt={attempt(
          'failed',
          { docker_check: { status: 'failed' } },
          {
            step: 'docker_check',
            reason: 'Docker is not responding',
            fix: { id: 'recheck_docker', label: 'Re-check Docker' },
          },
        )}
        onFix={onFix}
      />,
    )
    expect(screen.getByRole('alert').textContent).toContain(
      'Docker is not responding',
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Re-check Docker' }),
    )
    expect(onFix).toHaveBeenCalledWith('recheck_docker', false)
  })

  it('shows the ready state with next actions', () => {
    render(
      <ProvisioningView
        name="main"
        attempt={attempt('ready', { ready: { status: 'done' } })}
        onFix={vi.fn()}
      />,
    )
    expect(screen.getByText('provision.readyTitle')).toBeTruthy()
    expect(screen.getByText('provision.open')).toBeTruthy()
    expect(screen.getByText('provision.connectApp')).toBeTruthy()
  })
})
