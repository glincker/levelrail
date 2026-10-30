import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { AllI18n } from '../test/allI18n'
import {
  badgeMarkdown,
  brandingValid,
  type BrandingValues,
  componentBadgeUrl,
  isValidAccent,
  isValidLogoUrl,
  reorderGroups,
} from '../lib/statusPageHelpers'
import { StatusBrandingFields } from './StatusBrandingFields'

describe('status page helpers', () => {
  it('validates accent hex, empty allowed', () => {
    expect(isValidAccent('')).toBe(true)
    expect(isValidAccent('#3b5bdb')).toBe(true)
    expect(isValidAccent('#3b5')).toBe(false)
    expect(isValidAccent('red')).toBe(false)
  })

  it('accepts only https logo URLs without credentials', () => {
    expect(isValidLogoUrl('https://example.com/a.svg')).toBe(true)
    expect(isValidLogoUrl('http://example.com/a.svg')).toBe(false)
    expect(isValidLogoUrl('https://u:p@example.com/a.svg')).toBe(false)
    expect(isValidLogoUrl('not a url')).toBe(false)
  })

  it('moves a group and returns only renumbered groups', () => {
    const groups = [
      { id: 'a', name: 'A', position: 0 },
      { id: 'b', name: 'B', position: 1 },
      { id: 'c', name: 'C', position: 2 },
    ]
    const changed = reorderGroups(groups, 'c', -1)
    expect(changed.map((g) => [g.id, g.position])).toEqual([
      ['c', 1],
      ['b', 2],
    ])
    expect(reorderGroups(groups, 'a', -1)).toEqual([])
    expect(reorderGroups(groups, 'c', 1)).toEqual([])
  })

  it('builds markdown badge snippets', () => {
    const url = componentBadgeUrl('https://h/public/status/badge.svg', 'sc_1')
    expect(url).toBe('https://h/public/status/badge.svg?component=sc_1')
    expect(badgeMarkdown('https://h/public/status', url, 'API')).toBe(
      '[![API](https://h/public/status/badge.svg?component=sc_1)](https://h/public/status)',
    )
  })
})

const base: BrandingValues = {
  accent: '',
  logo_url: '',
  about: '',
  website: '',
  noindex: false,
  powered_by: true,
}

function Harness({ initial }: { initial: BrandingValues }) {
  const [v, setV] = useState(initial)
  return (
    <AllI18n>
      <StatusBrandingFields value={v} onChange={setV} />
      <output>{brandingValid(v) ? 'valid' : 'invalid'}</output>
    </AllI18n>
  )
}

describe('StatusBrandingFields', () => {
  it('flags a bad accent and a non-https logo inline', () => {
    render(<Harness initial={base} />)
    expect(screen.getByText('valid')).toBeTruthy()
    fireEvent.change(screen.getByLabelText('Accent colour'), {
      target: { value: 'blue' },
    })
    fireEvent.change(screen.getByLabelText('Logo URL'), {
      target: { value: 'http://example.com/a.png' },
    })
    expect(screen.getByText('invalid')).toBeTruthy()
    expect(screen.getAllByRole('alert')).toHaveLength(2)
  })

  it('shows the about character counter', () => {
    render(<Harness initial={{ ...base, about: 'hello' }} />)
    expect(screen.getByText('5 of 1000')).toBeTruthy()
  })
})
