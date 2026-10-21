import { cleanup, render } from '@testing-library/react'
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

describe('LogTerminal performance guard', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders 50,000 lines with only a window of rows in the DOM', () => {
    const lines: LogLine[] = Array.from({ length: 50_000 }, (_, i) => ({
      id: i,
      line: `line ${i}`,
      stream: 'stdout',
    }))
    const { container } = renderLines(lines)
    const rows = container.querySelectorAll('[data-index]')
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.length).toBeLessThan(500)
  })
})
