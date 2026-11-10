import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { setAppearance } from '../lib/appearance'
import { useListKeyboard } from './useListKeyboard'

function List({ onSelect }: { onSelect: () => void }) {
  const { setContainer } = useListKeyboard<HTMLDivElement>()
  return (
    <div>
      <div data-list-filter>
        <input aria-label="filter" />
      </div>
      <div ref={setContainer}>
        <table>
          <tbody>
            {['a', 'b'].map((r) => (
              <tr key={r} data-testid={`row-${r}`}>
                <td>
                  <input type="checkbox" aria-label={r} onChange={onSelect} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function key(k: string, target: Element = document.body) {
  fireEvent.keyDown(target, { key: k })
}

afterEach(() => {
  setAppearance({ listKeys: 'on' })
})

describe('useListKeyboard', () => {
  it('j moves the focused row and x toggles its selection', () => {
    Element.prototype.scrollIntoView = vi.fn()
    const onSelect = vi.fn()
    render(<List onSelect={onSelect} />)
    key('j')
    expect(document.activeElement).toBe(screen.getByTestId('row-a'))
    key('j')
    expect(document.activeElement).toBe(screen.getByTestId('row-b'))
    key('x')
    expect(onSelect).toHaveBeenCalledTimes(1)
  })

  it('/ focuses the filter', () => {
    render(<List onSelect={vi.fn()} />)
    key('/')
    expect(document.activeElement).toBe(screen.getByLabelText('filter'))
  })

  it('does nothing while typing in an input', () => {
    render(<List onSelect={vi.fn()} />)
    const input = screen.getByLabelText('filter')
    input.focus()
    key('j', input)
    expect(document.activeElement).toBe(input)
  })

  it('leaves j alone while a dialog is open outside the list', () => {
    render(
      <>
        <List onSelect={vi.fn()} />
        <div role="dialog" aria-label="open dialog" />
      </>,
    )
    key('j')
    expect(document.activeElement).toBe(document.body)
  })

  it('does nothing when the setting is off', () => {
    setAppearance({ listKeys: 'off' })
    render(<List onSelect={vi.fn()} />)
    key('j')
    key('/')
    expect(document.activeElement).toBe(document.body)
  })
})
