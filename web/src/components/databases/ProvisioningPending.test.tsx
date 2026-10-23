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
      o?.name ? `${k} ${o.name} ${o.size}` : k,
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
  failure?: ProvisionAttempt['failure'],
): ProvisionAttempt {
  return {
    attempt_id: 'prov_1',
    status,
    steps: STEPS.map((step) => ({
      step,
      status: status === 'ready' ? 'done' : 'pending',
    })),
    events: [],
    failure,
  }
}

describe('ProvisioningView for a database without a container', () => {
  afterEach(cleanup)

  it('offers the leftover volume choices with its name and size', async () => {
    const onFix = vi.fn()
    render(
      <ProvisioningView
        name="main"
        attempt={attempt('failed', {
          step: 'create_volume',
          reason: 'leftover volume db-main-data',
          volume: { name: 'db-main-data', size_bytes: 5242880 },
          fixes: [
            { id: 'reuse_existing_volume', label: 'Reuse the existing volume' },
            {
              id: 'remove_leftover_volume',
              label: 'Remove leftover volume and retry',
              confirm: true,
            },
            { id: 'choose_another_name', label: 'Choose another name' },
          ],
        })}
        onFix={onFix}
      />,
    )
    expect(screen.getByText(/db-main-data 5 MiB/)).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Remove leftover volume and retry' }),
    )
    expect(onFix).not.toHaveBeenCalled()
    await userEvent.click(
      screen.getByRole('button', { name: 'provision.confirmYes' }),
    )
    expect(onFix).toHaveBeenCalledWith('remove_leftover_volume', true)
  })

  it('edits the stored settings and discards after a confirm', async () => {
    const onSave = vi.fn()
    const onDiscard = vi.fn()
    render(
      <ProvisioningView
        name="main"
        attempt={attempt('failed', { step: 'start', reason: 'boom' })}
        onFix={vi.fn()}
        onSaveSettings={onSave}
        onDiscard={onDiscard}
        initialVersion="16"
      />,
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'provision.editSettings' }),
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'provision.saveAndRetry' }),
    )
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ version: '16' }),
    )

    await userEvent.click(
      screen.getByRole('button', { name: 'provision.discard' }),
    )
    expect(onDiscard).not.toHaveBeenCalled()
    await userEvent.click(
      screen.getByRole('button', { name: 'provision.discardYes' }),
    )
    expect(onDiscard).toHaveBeenCalledTimes(1)
  })

  it('shows host, port and a masked connection string when ready', () => {
    render(
      <ProvisioningView
        name="main"
        attempt={attempt('ready')}
        onFix={vi.fn()}
        connection={{ host: 'db-main', port: 5432, scheme: 'postgres' }}
      />,
    )
    expect(screen.getByText('db-main')).toBeTruthy()
    expect(screen.getByText('5432')).toBeTruthy()
    const url = screen.getByText(/postgres:\/\/user:/)
    expect(url.textContent).toContain('•')
    expect(screen.queryByRole('button', { name: /reveal/i })).toBeNull()
  })
})
