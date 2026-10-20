import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AppTerminal } from './AppTerminal'

// Only the execEnabled=false branch is covered here: the enabled path
// opens a real WebSocket and mounts @xterm/xterm against the DOM, which
// needs a live server and a layout engine jsdom doesn't provide, the
// same reasoning LogTerminal.test.tsx's own doc comment gives for
// skipping scroll-position behavior. The disabled branch returns before
// any of that runs (no session ever starts), so it's honestly testable
// here.
describe('AppTerminal', () => {
  it('shows the connect flow by default (execEnabled omitted)', () => {
    render(<AppTerminal name="demo-app" />)
    expect(
      screen.getByRole('button', { name: /start session/i }),
    ).toBeInTheDocument()
  })

  it('shows a disabled explanation instead of the connect flow when exec access is off', () => {
    render(<AppTerminal name="demo-app" execEnabled={false} />)

    expect(
      screen.getByText(/Shell\/exec access is disabled for this app/),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /start session/i }),
    ).not.toBeInTheDocument()
  })
})
