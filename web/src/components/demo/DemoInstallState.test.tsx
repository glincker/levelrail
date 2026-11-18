import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DemoInstallState } from './DemoInstallState'
import { DEMO_INSTALL_EVENT } from '@/lib/installPrompt'

describe('DemoInstallState', () => {
  it('dispatches the install event on click', async () => {
    const seen = vi.fn((e: Event) => {
      e.preventDefault()
    })
    window.addEventListener(DEMO_INSTALL_EVENT, seen)
    render(<DemoInstallState />)
    await userEvent.click(screen.getByRole('button'))
    window.removeEventListener(DEMO_INSTALL_EVENT, seen)
    expect(seen).toHaveBeenCalledTimes(1)
  })
})
