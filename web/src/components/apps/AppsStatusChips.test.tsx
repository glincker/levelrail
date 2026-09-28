import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AppsStatusChips, ViewToggle } from './AppsStatusChips'

const COUNTS = { running: 5, deploying: 1, failing: 2, stopped: 0 }

describe('AppsStatusChips', () => {
  it('shows a count per status and marks the active chip', () => {
    render(
      <AppsStatusChips counts={COUNTS} active="failing" onChange={vi.fn()} />,
    )
    expect(screen.getByRole('button', { name: /5\s*Running/ })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
    expect(screen.getByRole('button', { name: /2\s*Failing/ })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(
      screen.getByRole('button', { name: /0\s*Stopped/ }),
    ).toBeInTheDocument()
  })

  it('selects a status and toggles it off again', async () => {
    const onChange = vi.fn()
    const { rerender } = render(
      <AppsStatusChips counts={COUNTS} active={null} onChange={onChange} />,
    )
    await userEvent.click(screen.getByRole('button', { name: /Running/ }))
    expect(onChange).toHaveBeenLastCalledWith('running')
    rerender(
      <AppsStatusChips counts={COUNTS} active="running" onChange={onChange} />,
    )
    await userEvent.click(screen.getByRole('button', { name: /Running/ }))
    expect(onChange).toHaveBeenLastCalledWith(null)
  })
})

describe('ViewToggle', () => {
  it('reports the chosen mode', async () => {
    const onChange = vi.fn()
    render(<ViewToggle mode="table" onChange={onChange} />)
    expect(screen.getByRole('button', { name: 'Table view' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    await userEvent.click(screen.getByRole('button', { name: 'Card view' }))
    expect(onChange).toHaveBeenCalledWith('grid')
  })
})
