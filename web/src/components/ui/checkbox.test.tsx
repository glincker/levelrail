import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Checkbox } from './checkbox'
import { RadioGroup, RadioGroupItem } from './radio-group'

describe('Checkbox', () => {
  it('toggles and exposes the checked state', () => {
    render(<Checkbox aria-label="agree" />)
    const box = screen.getByRole('checkbox')
    expect(box.getAttribute('aria-checked')).toBe('false')
    fireEvent.click(box)
    expect(box.getAttribute('aria-checked')).toBe('true')
  })

  it('reports indeterminate as mixed', () => {
    render(<Checkbox aria-label="some" indeterminate />)
    expect(screen.getByRole('checkbox').getAttribute('aria-checked')).toBe(
      'mixed',
    )
  })

  it('ignores clicks when disabled', () => {
    render(<Checkbox aria-label="off" disabled />)
    const box = screen.getByRole('checkbox')
    fireEvent.click(box)
    expect(box.getAttribute('aria-checked')).toBe('false')
  })
})

describe('RadioGroup', () => {
  it('selects one item at a time', () => {
    render(
      <RadioGroup aria-label="size">
        <RadioGroupItem value="a" aria-label="a" />
        <RadioGroupItem value="b" aria-label="b" />
      </RadioGroup>,
    )
    const [a, b] = screen.getAllByRole('radio')
    fireEvent.click(a as HTMLElement)
    fireEvent.click(b as HTMLElement)
    expect(a?.getAttribute('aria-checked')).toBe('false')
    expect(b?.getAttribute('aria-checked')).toBe('true')
  })
})
