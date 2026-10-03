import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render } from '@testing-library/react'
import { useShortcuts } from './useShortcuts'
import { LONG_PRESS_MS } from '@/lib/shortcuts'

const navigate = vi.fn()
const experimentalFeatures = vi.fn<() => string[]>(() => [])

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))
vi.mock('./useExperimental', () => ({
  useExperimentalFeatures: () => experimentalFeatures(),
}))

function Harness({ onOpenStageOverlay }: { onOpenStageOverlay: () => void }) {
  useShortcuts({ onOpenStageOverlay })
  return <input aria-label="search" data-shortcut-search />
}

function pressL() {
  fireEvent.keyDown(window, { key: 'l' })
}
function releaseL() {
  fireEvent.keyUp(window, { key: 'l' })
}

describe('useShortcuts: long-press l opens the stage overlay', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    experimentalFeatures.mockReturnValue([])
  })
  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
    navigate.mockClear()
  })

  it('opens once the hold clears the threshold', () => {
    const onOpenStageOverlay = vi.fn()
    render(<Harness onOpenStageOverlay={onOpenStageOverlay} />)
    pressL()
    vi.advanceTimersByTime(LONG_PRESS_MS - 1)
    expect(onOpenStageOverlay).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(onOpenStageOverlay).toHaveBeenCalledTimes(1)
  })

  it('does nothing on a quick tap released before the threshold', () => {
    const onOpenStageOverlay = vi.fn()
    render(<Harness onOpenStageOverlay={onOpenStageOverlay} />)
    pressL()
    vi.advanceTimersByTime(LONG_PRESS_MS - 50)
    releaseL()
    vi.advanceTimersByTime(100)
    expect(onOpenStageOverlay).not.toHaveBeenCalled()
  })

  it('ignores synthetic key-repeat events while held', () => {
    const onOpenStageOverlay = vi.fn()
    render(<Harness onOpenStageOverlay={onOpenStageOverlay} />)
    fireEvent.keyDown(window, { key: 'l' })
    fireEvent.keyDown(window, { key: 'l', repeat: true })
    vi.advanceTimersByTime(LONG_PRESS_MS)
    expect(onOpenStageOverlay).toHaveBeenCalledTimes(1)
  })

  it('does not trigger while typing in a field', () => {
    const onOpenStageOverlay = vi.fn()
    render(<Harness onOpenStageOverlay={onOpenStageOverlay} />)
    const field = document.querySelector('input') as HTMLInputElement
    fireEvent.keyDown(field, { key: 'l' })
    vi.advanceTimersByTime(LONG_PRESS_MS)
    expect(onOpenStageOverlay).not.toHaveBeenCalled()
  })

  it('does not trigger while a dialog is already open', () => {
    const onOpenStageOverlay = vi.fn()
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('data-open', '')
    document.body.appendChild(dialog)
    render(<Harness onOpenStageOverlay={onOpenStageOverlay} />)
    pressL()
    vi.advanceTimersByTime(LONG_PRESS_MS)
    expect(onOpenStageOverlay).not.toHaveBeenCalled()
  })

  it('treats l as the second half of a g chord, not a long press', () => {
    experimentalFeatures.mockReturnValue(['load-balancer'])
    const onOpenStageOverlay = vi.fn()
    render(<Harness onOpenStageOverlay={onOpenStageOverlay} />)
    fireEvent.keyDown(window, { key: 'g' })
    pressL()
    vi.advanceTimersByTime(LONG_PRESS_MS)
    expect(onOpenStageOverlay).not.toHaveBeenCalled()
    expect(navigate).toHaveBeenCalledWith({ to: '/loadbalancers' })
  })

  it('cancels the pending hold when the window loses focus', () => {
    const onOpenStageOverlay = vi.fn()
    render(<Harness onOpenStageOverlay={onOpenStageOverlay} />)
    pressL()
    fireEvent(window, new Event('blur'))
    vi.advanceTimersByTime(LONG_PRESS_MS)
    expect(onOpenStageOverlay).not.toHaveBeenCalled()
  })

  it('still steps normal g-then-letter chords', () => {
    const onOpenStageOverlay = vi.fn()
    render(<Harness onOpenStageOverlay={onOpenStageOverlay} />)
    fireEvent.keyDown(window, { key: 'g' })
    fireEvent.keyDown(window, { key: 'a' })
    expect(navigate).toHaveBeenCalledWith({ to: '/apps' })
  })
})
