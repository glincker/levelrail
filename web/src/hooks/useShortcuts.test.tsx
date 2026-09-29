import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render } from '@testing-library/react'
import { useShortcuts } from './useShortcuts'

const navigate = vi.fn()

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))
vi.mock('./useExperimental', () => ({
  useExperimentalFeatures: () => [],
}))

function Harness({ onHelp }: { onHelp: () => void }) {
  useShortcuts({ onHelp })
  return <input aria-label="search" data-shortcut-search />
}

describe('useShortcuts', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('opens help on ?', () => {
    const onHelp = vi.fn()
    render(<Harness onHelp={onHelp} />)
    fireEvent.keyDown(window, { key: '?' })
    expect(onHelp).toHaveBeenCalledTimes(1)
  })

  it('does not open help while a dialog is actually open', () => {
    const onHelp = vi.fn()
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('data-open', '')
    document.body.appendChild(dialog)
    render(<Harness onHelp={onHelp} />)
    fireEvent.keyDown(window, { key: '?' })
    expect(onHelp).not.toHaveBeenCalled()
  })

  // Regression test: Base UI dialogs keep their DOM node mounted with
  // data-closed (not removed) during and after their exit animation. A
  // dialog in that state must not permanently disable every shortcut,
  // which is exactly the bug this hook used to have.
  it('still opens help when only a closed-but-mounted dialog exists', () => {
    const onHelp = vi.fn()
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('data-closed', '')
    document.body.appendChild(dialog)
    render(<Harness onHelp={onHelp} />)
    fireEvent.keyDown(window, { key: '?' })
    expect(onHelp).toHaveBeenCalledTimes(1)
  })
})
