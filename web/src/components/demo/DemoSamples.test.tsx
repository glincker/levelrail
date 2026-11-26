import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DemoGate } from './DemoUnavailable'
import { setDemoInfo } from '@/lib/demoStore'
import { DEMO_INSTALL_EVENT } from '@/lib/installPrompt'

describe('demo sample surfaces', () => {
  it('shows a labeled sample chat reply and an install action in demo mode', async () => {
    setDemoInfo({ enabled: true })
    const seen = vi.fn((e: Event) => {
      e.preventDefault()
    })
    window.addEventListener(DEMO_INSTALL_EVENT, seen)
    render(
      <DemoGate reason="ai">
        <p>real chat</p>
      </DemoGate>,
    )
    expect(screen.queryByText('real chat')).toBeNull()
    expect(screen.getByText('sample.badge')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'sample.chat.prompts.crashloop' }),
    )
    expect(screen.getByText('sample.chat.replies.crashloop')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'installState.install' }),
    )
    window.removeEventListener(DEMO_INSTALL_EVENT, seen)
    expect(seen).toHaveBeenCalledTimes(1)
    setDemoInfo({ enabled: false })
  })

  it('renders the real children outside demo mode', () => {
    setDemoInfo({ enabled: false })
    render(
      <DemoGate reason="database">
        <p>real console</p>
      </DemoGate>,
    )
    expect(screen.getByText('real console')).toBeTruthy()
  })

  it('shows sample terminal and query output for host surfaces', () => {
    setDemoInfo({ enabled: true })
    const { unmount } = render(<DemoGate reason="terminal">x</DemoGate>)
    expect(screen.getByLabelText('sample.terminal.label')).toBeTruthy()
    unmount()
    render(<DemoGate reason="database">x</DemoGate>)
    expect(screen.getByRole('table')).toBeTruthy()
    setDemoInfo({ enabled: false })
  })
})
