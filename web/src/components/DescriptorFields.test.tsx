import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DescriptorFields } from './DescriptorFields'
import { isFieldVisible, type FieldDescriptor } from '../types/fieldDescriptor'

const base = {
  type: 'string',
  required: false,
  secret: false,
  doc: '',
} as const

const fields: FieldDescriptor[] = [
  { ...base, id: 'a', key: 'a', label_key: 'la', help_key: 'ha' },
  {
    ...base,
    id: 'b',
    key: 'b',
    type: 'secret',
    secret: true,
    label_key: 'lb',
    help_key: 'hb',
    show_when: [{ field: 'a', equals: ['on'] }],
  },
]

const conditional = fields[1] as FieldDescriptor

describe('DescriptorFields', () => {
  it('shows a conditional field only when its condition holds', () => {
    expect(isFieldVisible(conditional, { a: 'off' })).toBe(false)
    expect(isFieldVisible(conditional, { a: 'on' })).toBe(true)
  })

  it('renders secrets as password inputs', () => {
    render(
      <DescriptorFields
        fields={fields}
        values={{ a: 'on' }}
        onChange={() => undefined}
        idPrefix="t"
      />,
    )
    expect(document.querySelector('#t-b')?.getAttribute('type')).toBe(
      'password',
    )
    expect(screen.queryByText('lb')).not.toBeNull()
  })
})
