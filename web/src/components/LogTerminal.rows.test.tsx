import { cleanup, fireEvent, render, screen } from '@testing-library/react'
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

  it('copies a single line from its hover button', () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    renderLines([{ id: 1, line: 'hello world', stream: 'stdout' }])
    fireEvent.click(screen.getByRole('button', { name: 'Copy this log line' }))
    expect(writeText).toHaveBeenCalledWith('hello world')
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
})
