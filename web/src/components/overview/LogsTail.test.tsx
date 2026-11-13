import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LogsTail } from './LogsTail'
import type { LogLine } from '../../hooks/useLogStream'

const state: { lines: LogLine[] } = { lines: [] }

vi.mock('../../hooks/useLogStream', () => ({
  useLogStream: () => ({ lines: state.lines, connectionState: 'open' }),
}))
vi.mock('../../hooks/use-mobile', () => ({ useIsMobile: () => false }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))

describe('LogsTail', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows the level as text, not only a dot, and does not clip long lines', () => {
    const long = `ERROR ${'y'.repeat(300)}`
    state.lines = [
      { id: 1, line: long, stream: 'stdout' },
      { id: 2, line: 'WARN disk low', stream: 'stdout' },
    ]
    render(<LogsTail appName="web" />)
    expect(screen.getByText('ERR')).toBeInTheDocument()
    expect(screen.getByText('WRN')).toBeInTheDocument()
    expect(
      screen.getByText('error', { selector: '.sr-only' }),
    ).toBeInTheDocument()
    const text = screen.getByText(long)
    expect(text).toHaveClass('whitespace-pre-wrap')
    expect(text).not.toHaveClass('truncate')
  })
})
