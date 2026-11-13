import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../test/commonI18n'
import { LogTerminal } from './LogTerminal'
import type { LogLine } from '../hooks/useLogStream'

// jsdom has no layout, so give every element a 400px viewport; otherwise
// the virtualizer sees a zero-height window and renders no rows at all.
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(400)
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(800)
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockReturnValue(
    new DOMRect(0, 0, 800, 400),
  )
})

function renderLines(lines: LogLine[]) {
  return render(
    <CommonI18n>
      <LogTerminal
        lines={lines}
        isPaused={false}
        pause={vi.fn()}
        resume={vi.fn()}
      />
    </CommonI18n>,
  )
}

describe('LogTerminal rows', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders plain selectable rows with no per-line copy or row buttons', () => {
    renderLines([{ id: 1, line: 'hello world', stream: 'stdout' }])
    expect(
      screen.queryByRole('button', { name: 'Copy this log line' }),
    ).toBeNull()
    const cell = screen.getByText('hello world')
    expect(cell).toHaveClass('select-text')
    expect(cell.closest('button')).toBeNull()
  })

  it('highlights filter matches with mark', () => {
    const { container } = renderLines([
      { id: 1, line: 'build failed', stream: 'stdout' },
      { id: 2, line: 'other', stream: 'stdout' },
    ])
    fireEvent.change(screen.getByLabelText('Filter log lines'), {
      target: { value: 'FAIL' },
    })
    const marks = container.querySelectorAll('mark')
    expect(marks).toHaveLength(1)
    expect(marks[0]?.textContent).toBe('fail')
  })

  describe('toolbar copy actions', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    beforeEach(() => {
      writeText.mockClear()
      Object.defineProperty(navigator, 'clipboard', {
        value: { writeText },
        configurable: true,
      })
    })

    it('Copy all visible copies every filtered line', async () => {
      renderLines([
        { id: 1, line: 'one', stream: 'stdout' },
        { id: 2, line: 'two', stream: 'stdout' },
      ])
      fireEvent.click(screen.getByRole('button', { name: /Copy all visible/ }))
      await waitFor(() => {
        expect(writeText).toHaveBeenCalledWith('one\ntwo')
      })
    })

    it('Copy selection falls back to the visible lines with no selection', async () => {
      renderLines([{ id: 1, line: 'one', stream: 'stdout' }])
      fireEvent.click(screen.getByRole('button', { name: /Copy selection/ }))
      await waitFor(() => {
        expect(writeText).toHaveBeenCalledWith('one')
      })
    })

    it('Copy selection copies only the selected text from the log region', async () => {
      renderLines([
        { id: 1, line: 'alpha', stream: 'stdout' },
        { id: 2, line: 'beta', stream: 'stdout' },
      ])
      const range = document.createRange()
      range.selectNodeContents(screen.getByText('beta'))
      const sel = window.getSelection()
      sel?.removeAllRanges()
      sel?.addRange(range)
      fireEvent.click(screen.getByRole('button', { name: /Copy selection/ }))
      await waitFor(() => {
        expect(writeText).toHaveBeenCalledWith('beta')
      })
      sel?.removeAllRanges()
    })
  })
})
