import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { RetentionDaysSelect } from './RetentionDaysSelect'

describe('RetentionDaysSelect floor', () => {
  it('disables positive options below the floor and keeps Never and longer ones', () => {
    render(
      <AllI18n>
        <RetentionDaysSelect
          value={365}
          allowed={[0, 30, 60, 90, 180, 365]}
          floor={90}
          ariaLabel="Keep audit log"
          onChange={vi.fn()}
        />
      </AllI18n>,
    )
    fireEvent.click(screen.getByRole('combobox', { name: 'Keep audit log' }))
    const disabled = (name: string) =>
      screen.getByRole('option', { name }).hasAttribute('data-disabled')
    expect(disabled('30 days')).toBe(true)
    expect(disabled('60 days')).toBe(true)
    expect(disabled('90 days')).toBe(false)
    expect(disabled('Never delete')).toBe(false)
  })
})
