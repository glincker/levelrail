import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { CloudInstance } from '../../types/cloudAccounts'
import { TerminateInstanceDialog } from './TerminateInstanceDialog'

const inst: CloudInstance = {
  id: 'i-1',
  name: 'web-1',
  region: 'eu-west-1',
  state: 'running',
  raw_state: 'running',
  shape: 't3.small',
  created_at: '2026-01-01T00:00:00Z',
  tags: {},
  managed: true,
  read_only: false,
}

function setup(canKeep: boolean) {
  const onConfirm = vi.fn()
  render(
    <TerminateInstanceDialog
      instance={inst}
      canKeepBootVolume={canKeep}
      pending={false}
      error={null}
      onOpenChange={vi.fn()}
      onConfirm={onConfirm}
    />,
  )
  return onConfirm
}

describe('TerminateInstanceDialog', () => {
  it('needs the typed name and keeps the boot volume off by default', async () => {
    const onConfirm = setup(true)
    const confirm = screen.getByRole('button', {
      name: 'instances.terminate.confirm',
    })
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/to confirm/), 'web-1')
    await userEvent.click(confirm)
    expect(onConfirm).toHaveBeenCalledWith(false)
  })

  it('sends keep when the box is ticked', async () => {
    const onConfirm = setup(true)
    await userEvent.click(screen.getByRole('checkbox'))
    await userEvent.type(screen.getByLabelText(/to confirm/), 'web-1')
    await userEvent.click(
      screen.getByRole('button', { name: 'instances.terminate.confirm' }),
    )
    expect(onConfirm).toHaveBeenCalledWith(true)
  })

  it('hides the keep option when the provider cannot keep it', () => {
    setup(false)
    expect(screen.queryByRole('checkbox')).toBeNull()
  })
})
