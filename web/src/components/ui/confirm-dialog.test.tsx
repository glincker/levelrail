import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ConfirmDialog } from './confirm-dialog'

function setup(
  props: Partial<React.ComponentProps<typeof ConfirmDialog>> = {},
) {
  const onConfirm = vi.fn()
  const onOpenChange = vi.fn()
  render(
    <ConfirmDialog
      open
      onOpenChange={onOpenChange}
      title="Delete web?"
      confirmLabel="Delete web"
      tone="destructive"
      onConfirm={onConfirm}
      {...props}
    />,
  )
  return { onConfirm, onOpenChange }
}

describe('ConfirmDialog', () => {
  it('confirms immediately when no typed value is required', async () => {
    const { onConfirm } = setup({ consequences: ['Containers are removed'] })
    expect(screen.getByText('Containers are removed')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Delete web' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('keeps confirm disabled until the exact name is typed', async () => {
    const { onConfirm } = setup({ requireTyped: 'web' })
    const confirm = screen.getByRole('button', { name: 'Delete web' })
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/to confirm/), 'we')
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/to confirm/), 'b')
    expect(confirm).toBeEnabled()
    await userEvent.click(confirm)
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('disables confirm and shows the pending label while pending', () => {
    setup({ pending: true, pendingLabel: 'Deleting...' })
    expect(screen.getByRole('button', { name: 'Deleting...' })).toBeDisabled()
  })

  it('shows an error message', () => {
    setup({ error: 'boom' })
    expect(screen.getByText('boom')).toBeInTheDocument()
  })
})
