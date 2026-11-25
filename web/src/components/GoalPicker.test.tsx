import { render as rtlRender, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { GoalPicker } from './GoalPicker'
import { DatabasesI18n } from '../test/databasesI18n'

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: DatabasesI18n })

describe('GoalPicker', () => {
  it('lists every goal and reports the chosen id', async () => {
    const onSelect = vi.fn()
    render(<GoalPicker onSelect={onSelect} />)
    expect(screen.getAllByRole('button')).toHaveLength(6)
    await userEvent.click(screen.getByRole('button', { name: /API/i }))
    expect(onSelect).toHaveBeenCalledWith('api')
  })

  it('shows only the database goal for a database scope', () => {
    render(<GoalPicker scope="databases" onSelect={vi.fn()} />)
    expect(screen.getAllByRole('button')).toHaveLength(1)
  })
})
