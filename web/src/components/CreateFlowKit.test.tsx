import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import {
  CreateFlowError,
  CreateFlowHeader,
  CreateFlowSteps,
  CreateFlowSubmitButton,
} from './CreateFlowKit'

// DialogTitle/DialogDescription (which CreateFlowHeader renders) require
// a surrounding Dialog.Root, the same as every real Create*Dialog
// already provides; this mirrors that context for a standalone unit
// test of the header piece.
function renderInDialog(children: ReactNode) {
  return render(
    <Dialog open modal={false}>
      <DialogContent showCloseButton={false}>{children}</DialogContent>
    </Dialog>,
  )
}

describe('CreateFlowHeader', () => {
  it('renders the icon, title, and description', () => {
    renderInDialog(
      <CreateFlowHeader
        icon={<span data-testid="icon" />}
        title="New thing"
        description="What this creates."
      />,
    )
    expect(screen.getByTestId('icon')).toBeInTheDocument()
    expect(screen.getByText('New thing')).toBeInTheDocument()
    expect(screen.getByText('What this creates.')).toBeInTheDocument()
  })

  it('renders without a description', () => {
    renderInDialog(<CreateFlowHeader title="New thing" />)
    expect(screen.getByText('New thing')).toBeInTheDocument()
  })
})

describe('CreateFlowSteps', () => {
  it('labels the current step for assistive tech', () => {
    render(
      <CreateFlowSteps
        steps={[
          { id: 'a', label: 'Details' },
          { id: 'b', label: 'Reveal' },
        ]}
        currentIndex={1}
      />,
    )
    expect(screen.getByLabelText('Step 2 of 2: Reveal')).toBeInTheDocument()
    expect(screen.getByText('Step 2 of 2: Reveal')).toBeInTheDocument()
  })
})

describe('CreateFlowError', () => {
  it('renders nothing when there is no message', () => {
    const { container } = render(<CreateFlowError message={null} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the message in a destructive alert when present', () => {
    render(<CreateFlowError message="Something went wrong." />)
    expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong.')
  })
})

describe('CreateFlowSubmitButton', () => {
  it('shows the idle label and is enabled', () => {
    render(
      <CreateFlowSubmitButton pending={false} pendingLabel="Saving...">
        Save
      </CreateFlowSubmitButton>,
    )
    const button = screen.getByRole('button', { name: 'Save' })
    expect(button).toBeEnabled()
  })

  it('swaps to the pending label and disables while pending', () => {
    render(
      <CreateFlowSubmitButton pending pendingLabel="Saving...">
        Save
      </CreateFlowSubmitButton>,
    )
    const button = screen.getByRole('button', { name: 'Saving...' })
    expect(button).toBeDisabled()
  })

  it('respects an explicit disabled prop even when not pending', async () => {
    const onClick = vi.fn()
    render(
      <CreateFlowSubmitButton
        pending={false}
        pendingLabel="Saving..."
        disabled
        onClick={onClick}
      >
        Save
      </CreateFlowSubmitButton>,
    )
    const user = userEvent.setup()
    const button = screen.getByRole('button', { name: 'Save' })
    expect(button).toBeDisabled()
    await user.click(button)
    expect(onClick).not.toHaveBeenCalled()
  })
})
