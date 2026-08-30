import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LogTerminal } from './LogTerminal'
import type { LogLine } from '../hooks/useLogStream'

// Scroll-position behavior (auto-scroll-to-bottom, pausing on manual
// scroll, isFinished stopping auto-scroll) is deliberately not covered
// here: jsdom has no layout engine, so scrollHeight/clientHeight/scrollTop
// are always 0 and any assertion about them would just be testing the
// mock, not real scroll behavior. The fullscreen toggle below is a real
// DOM/interaction change (a dialog mounts and unmounts) that jsdom can
// verify honestly.

const lines: LogLine[] = [
  { id: 1, line: 'starting build', stream: 'stdout' },
  { id: 2, line: 'build failed', stream: 'stderr' },
]

function renderTerminal() {
  const pause = vi.fn()
  const resume = vi.fn()
  render(
    <LogTerminal lines={lines} isPaused={false} pause={pause} resume={resume} />,
  )
  return { pause, resume }
}

describe('LogTerminal fullscreen toggle', () => {
  afterEach(() => {
    cleanup()
  })

  it('has no dialog mounted by default', () => {
    renderTerminal()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('expands into a dialog and collapses back on toggle', async () => {
    const user = userEvent.setup()
    renderTerminal()

    await user.click(screen.getByRole('button', { name: 'View fullscreen' }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Exit fullscreen' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
