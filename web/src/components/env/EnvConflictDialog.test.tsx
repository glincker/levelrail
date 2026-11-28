import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { EnvConflictDialog } from './EnvConflictDialog'

describe('EnvConflictDialog', () => {
  it('keeps every key by default and reports the chosen ones', () => {
    const onKeepMine = vi.fn()
    render(
      <EnvConflictDialog
        open
        keys={['A', 'TOKEN']}
        secretKeys={['TOKEN']}
        saving={false}
        onKeepMine={onKeepMine}
        onReloadTheirs={vi.fn()}
        onClose={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByLabelText('A'))
    fireEvent.click(screen.getByText('conflict.keepMine'))
    expect(onKeepMine).toHaveBeenCalledWith(['TOKEN'])
  })

  it('reload theirs never calls keep mine', () => {
    const onKeepMine = vi.fn()
    const onReload = vi.fn()
    render(
      <EnvConflictDialog
        open
        keys={['A']}
        secretKeys={[]}
        saving={false}
        onKeepMine={onKeepMine}
        onReloadTheirs={onReload}
        onClose={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByText('conflict.reload'))
    expect(onReload).toHaveBeenCalled()
    expect(onKeepMine).not.toHaveBeenCalled()
  })
})
