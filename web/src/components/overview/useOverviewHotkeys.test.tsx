import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render } from '@testing-library/react'
import { useOverviewHotkeys } from './useOverviewHotkeys'

function Harness({
  onOpenApp,
  onCopyUrl,
  onDeploy,
}: {
  onOpenApp: () => void
  onCopyUrl: () => void
  onDeploy: () => void
}) {
  useOverviewHotkeys({ onOpenApp, onCopyUrl, onDeploy })
  return null
}

describe('useOverviewHotkeys', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('dispatches o, c, and D to their handlers', () => {
    const onOpenApp = vi.fn()
    const onCopyUrl = vi.fn()
    const onDeploy = vi.fn()
    render(
      <Harness
        onOpenApp={onOpenApp}
        onCopyUrl={onCopyUrl}
        onDeploy={onDeploy}
      />,
    )
    fireEvent.keyDown(window, { key: 'o' })
    fireEvent.keyDown(window, { key: 'c' })
    fireEvent.keyDown(window, { key: 'D' })
    expect(onOpenApp).toHaveBeenCalledTimes(1)
    expect(onCopyUrl).toHaveBeenCalledTimes(1)
    expect(onDeploy).toHaveBeenCalledTimes(1)
  })

  it('is blocked while a dialog is actually open', () => {
    const onOpenApp = vi.fn()
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('data-open', '')
    document.body.appendChild(dialog)
    render(
      <Harness onOpenApp={onOpenApp} onCopyUrl={vi.fn()} onDeploy={vi.fn()} />,
    )
    fireEvent.keyDown(window, { key: 'o' })
    expect(onOpenApp).not.toHaveBeenCalled()
  })

  // Regression test: Base UI dialogs keep their DOM node mounted with
  // data-closed (not removed) during and after their exit animation. A
  // dialog in that state must not permanently disable every hotkey.
  it('is not blocked by a closed dialog that stays mounted for its exit animation', () => {
    const onOpenApp = vi.fn()
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('data-closed', '')
    document.body.appendChild(dialog)
    render(
      <Harness onOpenApp={onOpenApp} onCopyUrl={vi.fn()} onDeploy={vi.fn()} />,
    )
    fireEvent.keyDown(window, { key: 'o' })
    expect(onOpenApp).toHaveBeenCalledTimes(1)
  })
})
