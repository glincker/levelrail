import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LogToolbar } from './LogToolbar'
import type { LogFilter } from '../lib/logFilter'

function setup(filter: LogFilter = { text: '', stderrOnly: false }, shown = 5) {
  const onFilterChange = vi.fn()
  const onCopy = vi.fn(() => Promise.resolve())
  const onJumpToError = vi.fn()
  render(
    <LogToolbar
      filter={filter}
      onFilterChange={onFilterChange}
      shown={shown}
      total={5}
      onCopy={onCopy}
      onDownload={vi.fn()}
      onJumpToError={onJumpToError}
    />,
  )
  return { onFilterChange, onCopy, onJumpToError }
}

describe('LogToolbar', () => {
  afterEach(() => {
    cleanup()
  })

  it('marks the active level chip as pressed', () => {
    setup({ text: '', stderrOnly: false, level: 'warnings' })
    expect(screen.getByRole('button', { name: 'Warnings' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByRole('button', { name: 'All' })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
    expect(screen.getByRole('group', { name: 'Log level' })).toBeInTheDocument()
  })

  it('changes the level and clears stderrOnly', async () => {
    const { onFilterChange } = setup({ text: 'x', stderrOnly: true })
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: 'Errors' }))
    expect(onFilterChange).toHaveBeenCalledWith({
      text: 'x',
      stderrOnly: false,
      level: 'errors',
    })
  })

  it('emits text filter changes', async () => {
    const { onFilterChange } = setup()
    await userEvent.setup().type(screen.getByLabelText('Filter log lines'), 'a')
    expect(onFilterChange).toHaveBeenCalledWith({
      text: 'a',
      stderrOnly: false,
    })
  })

  it('shows the filtered count in a status region', () => {
    setup(undefined, 2)
    expect(screen.getByRole('status')).toHaveTextContent('2 of 5 lines')
  })

  it('announces a copy and calls jump to error', async () => {
    const { onCopy, onJumpToError } = setup()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /Copy/ }))
    expect(onCopy).toHaveBeenCalledOnce()
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Copied to clipboard',
    )
    await user.click(
      screen.getByRole('button', { name: /Jump to first error/ }),
    )
    expect(onJumpToError).toHaveBeenCalledOnce()
  })
})
