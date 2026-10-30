import { describe, expect, it } from 'vitest'
import { formatMonthlySaving } from './recommendationFormat'

describe('formatMonthlySaving', () => {
  it('formats a positive saving', () => {
    expect(formatMonthlySaving(4.5)).toBe('$4.50')
  })
  it('hides zero, negative and missing values', () => {
    expect(formatMonthlySaving(0)).toBeNull()
    expect(formatMonthlySaving(-1)).toBeNull()
    expect(formatMonthlySaving(undefined)).toBeNull()
  })
})
