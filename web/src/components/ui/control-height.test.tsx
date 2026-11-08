import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Input } from './input'
import { Select, SelectTrigger, SelectValue } from './select'
import { Textarea } from './textarea'
import { Badge } from './badge'

const CONTROL_H = 'h-(--glinr-control-h)'

describe('control height token', () => {
  it('gives Input and Select the same height class', () => {
    const { container } = render(
      <div>
        <Input aria-label="name" />
        <Select>
          <SelectTrigger aria-label="kind">
            <SelectValue />
          </SelectTrigger>
        </Select>
      </div>,
    )
    const input = container.querySelector('[data-slot="input"]')
    const trigger = container.querySelector('[data-slot="select-trigger"]')
    expect(input?.className).toContain(CONTROL_H)
    expect(trigger?.className).toContain(`data-[size=default]:${CONTROL_H}`)
  })

  it('shares the control radius token across Input and Textarea', () => {
    const { container } = render(
      <div>
        <Input aria-label="a" />
        <Textarea aria-label="b" />
      </div>,
    )
    for (const slot of ['input', 'textarea']) {
      expect(
        container.querySelector(`[data-slot="${slot}"]`)?.className,
      ).toContain('rounded-(--glinr-radius-control)')
    }
  })

  it('sizes the badge from the badge height token', () => {
    const { getByText } = render(<Badge>ok</Badge>)
    expect(getByText('ok').className).toContain('h-(--glinr-badge-h)')
  })
})
