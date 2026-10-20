import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StepFooter } from './StepChrome'

describe('StepFooter', () => {
  it('disables Continue and explains why when the gate is closed', () => {
    const onContinue = vi.fn()
    render(
      <StepFooter
        gate={{
          canContinue: false,
          reason: 'Waiting for DNS: not resolving yet.',
        }}
        onContinue={onContinue}
        onSkip={vi.fn()}
      />,
    )
    const button = screen.getByRole('button', { name: /continue/i })
    expect(button).toBeDisabled()
    expect(button).toHaveAccessibleDescription(
      'Waiting for DNS: not resolving yet.',
    )
    fireEvent.click(button)
    expect(onContinue).not.toHaveBeenCalled()
  })

  it('enables Continue with no reason once the gate opens', () => {
    const onContinue = vi.fn()
    render(<StepFooter gate={{ canContinue: true }} onContinue={onContinue} />)
    const button = screen.getByRole('button', { name: /continue/i })
    expect(button).toBeEnabled()
    fireEvent.click(button)
    expect(onContinue).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: /skip/i })).toBeNull()
  })

  it('offers Skip on optional steps', () => {
    const onSkip = vi.fn()
    render(
      <StepFooter
        gate={{ canContinue: false, reason: 'x' }}
        onContinue={vi.fn()}
        onSkip={onSkip}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /skip this step/i }))
    expect(onSkip).toHaveBeenCalledOnce()
  })
})
