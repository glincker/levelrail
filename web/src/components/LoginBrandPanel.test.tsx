import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LoginBrandPanel } from './LoginBrandPanel'

function activeEyebrow(container: HTMLElement): string | null {
  const active = container.querySelector('[aria-hidden="false"]')
  return active?.querySelector('p')?.textContent ?? null
}

describe('LoginBrandPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('rotates through the slides on a timer', () => {
    const { container } = render(<LoginBrandPanel name="Levelrail" />)
    expect(activeEyebrow(container)).toBe('Self-hosted deploys')
    act(() => {
      vi.advanceTimersByTime(7000)
    })
    expect(activeEyebrow(container)).toBe('Data next door')
  })

  it('jumps to a slide from the progress control', () => {
    const { container } = render(<LoginBrandPanel name="Levelrail" />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Show: Secure by default' }),
    )
    expect(activeEyebrow(container)).toBe('Secure by default')
  })
})
