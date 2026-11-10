import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { AllI18n } from '../test/allI18n'
import { FilterChips } from './FilterChips'

const ROWS = ['web', 'api', 'worker']

function Harness() {
  const [q, setQ] = useState('nope')
  const shown = ROWS.filter((r) => r.includes(q))
  return (
    <>
      <FilterChips
        filters={
          q
            ? [
                {
                  id: 'q',
                  label: `Search: ${q}`,
                  onRemove: () => {
                    setQ('')
                  },
                },
              ]
            : []
        }
        shown={shown.length}
        total={ROWS.length}
        onClearAll={() => {
          setQ('')
        }}
      />
      <ul>
        {shown.map((r) => (
          <li key={r}>{r}</li>
        ))}
      </ul>
    </>
  )
}

describe('FilterChips', () => {
  it('shows the chip row on a filter with no matches and clear-all restores rows', () => {
    render(
      <AllI18n>
        <Harness />
      </AllI18n>,
    )
    expect(screen.getByRole('group', { name: 'Active filters' })).toBeTruthy()
    expect(screen.getByText('Search: nope')).toBeTruthy()
    expect(screen.getByText('Nothing matches these filters')).toBeTruthy()
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
    const [clear] = screen.getAllByRole('button', { name: 'Clear all filters' })
    if (!clear) throw new Error('clear-all button missing')
    fireEvent.click(clear)
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
    expect(screen.queryByRole('group', { name: 'Active filters' })).toBeNull()
  })
})
