import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RecentBubbles, type Bubble } from './RecentBubbles'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const bubble = (label: string): Bubble => ({
  key: label,
  label,
  icon: <svg />,
  onPick: vi.fn(),
})

describe('RecentBubbles', () => {
  it('renders nothing without bubbles', () => {
    const { container } = render(<RecentBubbles bubbles={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('exposes each bubble as a named button with its Alt+number shortcut', () => {
    render(<RecentBubbles bubbles={[bubble('Apps'), bubble('Backups')]} />)
    expect(screen.getByRole('button', { name: 'Apps' })).toHaveAttribute(
      'aria-keyshortcuts',
      'Alt+1',
    )
    expect(screen.getByRole('button', { name: 'Backups' })).toHaveAttribute(
      'aria-keyshortcuts',
      'Alt+2',
    )
  })

  it('navigates when a bubble is clicked', async () => {
    const b = bubble('Apps')
    render(<RecentBubbles bubbles={[b]} />)
    await userEvent.click(screen.getByRole('button', { name: 'Apps' }))
    expect(b.onPick).toHaveBeenCalledOnce()
  })
})

describe('RecentBubbles shortcut labels', () => {
  const setPlatform = (value: string) =>
    vi.spyOn(window.navigator, 'platform', 'get').mockReturnValue(value)

  it('shows Option on Mac and Alt elsewhere in the tooltip', async () => {
    for (const [platform, label] of [
      ['MacIntel', '⌥1'],
      ['Win32', 'Alt+1'],
    ] as const) {
      const spy = setPlatform(platform)
      const { unmount } = render(<RecentBubbles bubbles={[bubble('Apps')]} />)
      await userEvent.hover(screen.getByRole('button', { name: 'Apps' }))
      expect(await screen.findByText(label)).toBeInTheDocument()
      unmount()
      spy.mockRestore()
    }
  })

  it('dims bubbles that do not match the query but keeps them clickable', () => {
    render(
      <RecentBubbles
        bubbles={[bubble('Apps'), bubble('Backups')]}
        query="back"
      />,
    )
    expect(screen.getByRole('button', { name: 'Apps' })).toHaveClass(
      'opacity-60',
    )
    expect(screen.getByRole('button', { name: 'Backups' })).not.toHaveClass(
      'opacity-60',
    )
  })
})

describe('RecentBubbles reorder', () => {
  const rects = () =>
    vi
      .spyOn(HTMLElement.prototype, 'getBoundingClientRect')
      .mockImplementation(function (this: HTMLElement) {
        const el = this.closest('[data-bubble]')
        const idx = el
          ? Array.from(el.parentElement?.children ?? [])
              .filter((c) => c.hasAttribute('data-bubble'))
              .indexOf(el)
          : 0
        const left = Math.max(0, idx) * 50
        return {
          left,
          right: left + 44,
          width: 44,
          height: 44,
          top: 0,
          bottom: 44,
          x: left,
          y: 0,
          toJSON: () => ({}),
        }
      })
  const move = (el: Element, fromX: number, toX: number) => {
    fireEvent.pointerDown(el, {
      clientX: fromX,
      button: 0,
      pointerId: 1,
      pointerType: 'mouse',
    })
    fireEvent.pointerMove(el, {
      clientX: toX,
      pointerId: 1,
      pointerType: 'mouse',
    })
    fireEvent.pointerUp(el, {
      clientX: toX,
      pointerId: 1,
      pointerType: 'mouse',
    })
  }
  const three = () => [bubble('A'), bubble('B'), bubble('C')]
  afterEach(() => vi.restoreAllMocks())

  it('reorders when dragged past the threshold and does not open it', () => {
    rects()
    const onReorder = vi.fn()
    const list = three()
    render(<RecentBubbles bubbles={list} onReorder={onReorder} />)
    const a = screen.getByRole('button', { name: 'A' })
    move(a, 0, 55)
    expect(onReorder).toHaveBeenCalledOnce()
    const [next, moved] = onReorder.mock.calls[0] as [Bubble[], string]
    expect(next.map((b) => b.label)).toEqual(['B', 'A', 'C'])
    expect(moved).toBe('A')
    fireEvent.click(a)
    expect(list[0]?.onPick).not.toHaveBeenCalled()
  })

  it('treats a tiny move as a click', () => {
    rects()
    const onReorder = vi.fn()
    const list = three()
    render(<RecentBubbles bubbles={list} onReorder={onReorder} />)
    const a = screen.getByRole('button', { name: 'A' })
    move(a, 0, 2)
    fireEvent.click(a)
    expect(onReorder).not.toHaveBeenCalled()
    expect(list[0]?.onPick).toHaveBeenCalledOnce()
  })

  it('mirrors the drag direction in RTL', () => {
    rects()
    const onReorder = vi.fn()
    render(
      <div dir="rtl">
        <RecentBubbles bubbles={three()} onReorder={onReorder} />
      </div>,
    )
    move(screen.getByRole('button', { name: 'A' }), 0, -55)
    const [next] = onReorder.mock.calls[0] as [Bubble[]]
    expect(next.map((b) => b.label)).toEqual(['B', 'A', 'C'])
  })

  it('lifts with Space, moves with arrows, drops with Space and announces', async () => {
    const onReorder = vi.fn()
    render(<RecentBubbles bubbles={three()} onReorder={onReorder} />)
    const a = screen.getByRole('button', { name: 'A' })
    a.focus()
    await userEvent.keyboard(' ')
    await userEvent.keyboard('{ArrowRight}')
    expect(await screen.findByText('bubbles.moved')).toBeInTheDocument()
    await userEvent.keyboard(' ')
    const [next, moved] = onReorder.mock.calls[0] as [Bubble[], string]
    expect(next.map((b) => b.label)).toEqual(['B', 'A', 'C'])
    expect(moved).toBe('A')
  })

  it('cancels a lift with Escape and commits nothing', async () => {
    const onReorder = vi.fn()
    render(<RecentBubbles bubbles={three()} onReorder={onReorder} />)
    screen.getByRole('button', { name: 'A' }).focus()
    await userEvent.keyboard(' {ArrowRight}{Escape}')
    expect(onReorder).not.toHaveBeenCalled()
    expect(
      screen
        .getAllByRole('button', { name: /^[ABC]$/ })
        .map((b) => b.getAttribute('aria-label')),
    ).toEqual(['A', 'B', 'C'])
  })

  it('swaps arrow meaning in RTL while lifted', async () => {
    const onReorder = vi.fn()
    render(
      <div dir="rtl">
        <RecentBubbles bubbles={three()} onReorder={onReorder} />
      </div>,
    )
    screen.getByRole('button', { name: 'A' }).focus()
    await userEvent.keyboard(' {ArrowLeft} ')
    const [next] = onReorder.mock.calls[0] as [Bubble[]]
    expect(next.map((b) => b.label)).toEqual(['B', 'A', 'C'])
  })

  it('keeps Enter as open', async () => {
    const list = three()
    render(<RecentBubbles bubbles={list} />)
    screen.getByRole('button', { name: 'B' }).focus()
    await userEvent.keyboard('{Enter}')
    expect(list[1]?.onPick).toHaveBeenCalledOnce()
  })

  it('shows the pin mark only on pinned bubbles', () => {
    render(
      <RecentBubbles
        bubbles={[{ ...bubble('A'), pinned: true }, bubble('B')]}
      />,
    )
    expect(screen.getAllByTestId('bubble-pin-mark')).toHaveLength(1)
  })
})

describe('RecentBubbles context menu', () => {
  const withHref = (label: string, pinned = false): Bubble => ({
    ...bubble(label),
    pinned,
    href: `/${label}`,
  })

  it('opens on right click with unpin, move and new tab actions', async () => {
    const onUnpin = vi.fn()
    const onReorder = vi.fn()
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    render(
      <RecentBubbles
        bubbles={[withHref('A', true), withHref('B')]}
        onUnpin={onUnpin}
        onReorder={onReorder}
      />,
    )
    fireEvent.contextMenu(screen.getByRole('button', { name: 'A' }))
    expect(
      await screen.findByRole('menuitem', { name: /bubbles.moveLeft/ }),
    ).toHaveAttribute('aria-disabled', 'true')
    await userEvent.click(
      screen.getByRole('menuitem', { name: /bubbles.openNewTab/ }),
    )
    expect(open).toHaveBeenCalledWith('/A', '_blank', 'noopener,noreferrer')

    fireEvent.contextMenu(screen.getByRole('button', { name: 'A' }))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: /bubbles.moveRight/ }),
    )
    expect(
      (onReorder.mock.calls[0] as [Bubble[]])[0].map((b) => b.label),
    ).toEqual(['B', 'A'])

    fireEvent.contextMenu(screen.getByRole('button', { name: 'A' }))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: /bubbles.unpin/ }),
    )
    expect(onUnpin).toHaveBeenCalledWith('A')
  })

  it('omits unpin for automatic bubbles and opens with Shift+F10, closing on Escape with focus back', async () => {
    render(<RecentBubbles bubbles={[withHref('A')]} />)
    const a = screen.getByRole('button', { name: 'A' })
    a.focus()
    await userEvent.keyboard('{Shift>}{F10}{/Shift}')
    expect(
      await screen.findByRole('menuitem', { name: /bubbles.moveLeft/ }),
    ).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: /bubbles.unpin/ })).toBeNull()
    await userEvent.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull())
    await waitFor(() => expect(a).toHaveFocus())
  })

  it('opens on a long touch press', () => {
    vi.useFakeTimers()
    render(<RecentBubbles bubbles={[withHref('A')]} />)
    fireEvent.pointerDown(screen.getByRole('button', { name: 'A' }), {
      button: 0,
      pointerId: 2,
      pointerType: 'touch',
      clientX: 5,
    })
    act(() => {
      vi.advanceTimersByTime(600)
    })
    vi.useRealTimers()
    expect(screen.getByRole('button', { name: 'A' })).toBeInTheDocument()
    expect(
      document.querySelector('[data-slot="dropdown-menu-content"]'),
    ).not.toBeNull()
  })
})
