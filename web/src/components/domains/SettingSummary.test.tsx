import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { SettingSummary } from './SettingSummary'

function Harness() {
  const [editing, setEditing] = useState(false)
  return (
    <SettingSummary
      status={<span>On</span>}
      summary="10 req/s"
      editLabel="Edit"
      closeLabel="Close"
      editing={editing}
      onEditingChange={setEditing}
    >
      <input aria-label="Limit" />
    </SettingSummary>
  )
}

describe('SettingSummary', () => {
  it('shows state always and the editor only after Edit', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    expect(screen.getByText('On')).toBeInTheDocument()
    expect(screen.getByText('10 req/s')).toBeInTheDocument()
    expect(screen.queryByLabelText('Limit')).not.toBeInTheDocument()
    const btn = screen.getByRole('button', { name: 'Edit' })
    expect(btn).toHaveAttribute('aria-expanded', 'false')
    await user.click(btn)
    expect(screen.getByLabelText('Limit')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Close' })).toHaveAttribute(
      'aria-expanded',
      'true',
    )
  })
})
