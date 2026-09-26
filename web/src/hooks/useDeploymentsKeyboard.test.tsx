import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render } from '@testing-library/react'
import { useDeploymentsKeyboard } from './useDeploymentsKeyboard'
import type { DeploymentKeyAction } from '../lib/deploymentsKeyboard'

function Harness({
  onAction,
  drawerOpen = false,
}: {
  onAction: (a: DeploymentKeyAction) => void
  drawerOpen?: boolean
}) {
  useDeploymentsKeyboard(drawerOpen, onAction)
  return (
    <div>
      <input aria-label="search" />
      <button type="button">plain</button>
      <button type="button" data-deployment-id="d1">
        row
      </button>
    </div>
  )
}

describe('useDeploymentsKeyboard', () => {
  it('dispatches j and k as row moves', () => {
    const onAction = vi.fn<(a: DeploymentKeyAction) => void>()
    render(<Harness onAction={onAction} />)
    fireEvent.keyDown(window, { key: 'j' })
    fireEvent.keyDown(window, { key: 'k' })
    expect(onAction.mock.calls.map((c) => c[0])).toEqual([
      { type: 'move', delta: 1 },
      { type: 'move', delta: -1 },
    ])
  })

  it('stays quiet while typing in an input', () => {
    const onAction = vi.fn<(a: DeploymentKeyAction) => void>()
    const { getByLabelText } = render(<Harness onAction={onAction} />)
    fireEvent.keyDown(getByLabelText('search'), { key: 'r' })
    expect(onAction).not.toHaveBeenCalled()
  })

  it('lets a focused control keep Enter but opens from a row', () => {
    const onAction = vi.fn<(a: DeploymentKeyAction) => void>()
    const { getByText } = render(<Harness onAction={onAction} />)
    fireEvent.keyDown(getByText('plain'), { key: 'Enter' })
    expect(onAction).not.toHaveBeenCalled()
    fireEvent.keyDown(getByText('row'), { key: 'Enter' })
    expect(onAction).toHaveBeenCalledTimes(1)
  })

  it('closes the drawer on Escape only while it is open', () => {
    const onAction = vi.fn<(a: DeploymentKeyAction) => void>()
    const { rerender } = render(<Harness onAction={onAction} />)
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onAction).not.toHaveBeenCalled()
    rerender(<Harness onAction={onAction} drawerOpen />)
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onAction.mock.calls[0]?.[0]).toEqual({ type: 'close' })
  })

  it('is blocked while a confirm dialog outside the drawer is open', () => {
    const onAction = vi.fn<(a: DeploymentKeyAction) => void>()
    render(<Harness onAction={onAction} />)
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    document.body.appendChild(dialog)
    fireEvent.keyDown(window, { key: 'c' })
    dialog.remove()
    expect(onAction).not.toHaveBeenCalled()
  })

  it('still works while only the drawer dialog is open', () => {
    const onAction = vi.fn<(a: DeploymentKeyAction) => void>()
    render(<Harness onAction={onAction} drawerOpen />)
    const drawer = document.createElement('div')
    drawer.setAttribute('data-deployment-drawer', '')
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    drawer.appendChild(dialog)
    document.body.appendChild(drawer)
    fireEvent.keyDown(window, { key: 'j' })
    drawer.remove()
    expect(onAction).toHaveBeenCalledTimes(1)
  })
})
