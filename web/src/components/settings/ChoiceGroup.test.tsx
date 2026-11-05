import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { ChoiceGroup, type Choice } from './ChoiceGroup'

const options: Choice<'a' | 'b' | 'c'>[] = [
  { value: 'a', label: 'Alpha', hint: 'first', isDefault: true },
  { value: 'b', label: 'Beta', hint: 'second', isRecommended: true },
  { value: 'c', label: 'Gamma', sample: <i data-testid="sample" /> },
]

function Harness({ onChange }: { onChange?: (v: string) => void }) {
  const [v, setV] = useState<'a' | 'b' | 'c'>('a')
  return (
    <ChoiceGroup
      label="Pick"
      value={v}
      options={options}
      onChange={(n) => {
        setV(n)
        onChange?.(n)
      }}
      defaultLabel="Default"
      recommendedLabel="Recommended"
    />
  )
}

describe('ChoiceGroup', () => {
  it('selects on click and keeps a roving tabindex', () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    fireEvent.click(screen.getByRole('radio', { name: /^Beta/ }))
    expect(onChange).toHaveBeenCalledWith('b')
    expect(
      screen.getByRole('radio', { name: /^Beta/ }).getAttribute('aria-checked'),
    ).toBe('true')
    expect(
      screen.getByRole('radio', { name: /^Alpha/ }).getAttribute('tabindex'),
    ).toBe('-1')
  })

  it('moves selection with the arrow keys and wraps', () => {
    render(<Harness />)
    const alpha = screen.getByRole('radio', { name: /^Alpha/ })
    fireEvent.keyDown(alpha, { key: 'ArrowLeft' })
    expect(
      screen
        .getByRole('radio', { name: /^Gamma/ })
        .getAttribute('aria-checked'),
    ).toBe('true')
    fireEvent.keyDown(screen.getByRole('radio', { name: /^Gamma/ }), {
      key: 'ArrowRight',
    })
    expect(alpha.getAttribute('aria-checked')).toBe('true')
  })

  it('shows Default and Recommended chips and a sample', () => {
    render(<Harness />)
    expect(screen.getByText('Default')).toBeTruthy()
    expect(screen.getByText('Recommended')).toBeTruthy()
    expect(screen.getByTestId('sample')).toBeTruthy()
    expect(screen.getByRole('radiogroup', { name: 'Pick' })).toBeTruthy()
  })
})
