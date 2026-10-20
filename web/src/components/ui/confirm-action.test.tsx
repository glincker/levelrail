import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ConfirmAction, type ConfirmActionProps } from './confirm-action'

interface Case {
  name: string
  props: Partial<ConfirmActionProps>
  typed?: string
  confirmEnabled: boolean
  visible?: string
}

const cases: Case[] = [
  {
    name: 'plain confirm is enabled',
    props: {},
    confirmEnabled: true,
  },
  {
    name: 'typed confirm is disabled until the value is typed',
    props: { typedConfirm: 'web' },
    confirmEnabled: false,
  },
  {
    name: 'typed confirm enables on exact match',
    props: { typedConfirm: 'web' },
    typed: 'web',
    confirmEnabled: true,
  },
  {
    name: 'pending disables confirm',
    props: { pending: true },
    confirmEnabled: false,
  },
  {
    name: 'error is surfaced',
    props: { error: 'Boom' },
    confirmEnabled: true,
    visible: 'Boom',
  },
]

function renderAction(props: Partial<ConfirmActionProps>) {
  render(
    <ConfirmAction
      trigger={(open) => (
        <button type="button" onClick={open}>
          Open
        </button>
      )}
      title="Delete web?"
      consequences={['Gone for good']}
      confirmLabel="Delete web"
      onConfirm={vi.fn()}
      {...props}
    />,
  )
}

describe('ConfirmAction', () => {
  it.each(cases)('$name', async (c) => {
    renderAction(c.props)
    await userEvent.click(screen.getByRole('button', { name: 'Open' }))
    if (c.typed) {
      await userEvent.type(screen.getByLabelText(/to confirm/), c.typed)
    }
    const confirm = screen.getByRole('button', { name: 'Delete web' })
    expect(confirm.hasAttribute('disabled')).toBe(!c.confirmEnabled)
    if (c.visible) expect(screen.getByText(c.visible)).toBeInTheDocument()
    expect(screen.getByText('Gone for good')).toBeInTheDocument()
  })

  it('passes a close callback and reports open changes', async () => {
    const onOpenChange = vi.fn()
    renderAction({
      onOpenChange,
      onConfirm: (close) => {
        close()
      },
    })
    await userEvent.click(screen.getByRole('button', { name: 'Open' }))
    await userEvent.click(screen.getByRole('button', { name: 'Delete web' }))
    expect(onOpenChange).toHaveBeenNthCalledWith(1, true)
    expect(onOpenChange).toHaveBeenLastCalledWith(false)
  })
})
