import { describe, expect, it } from 'vitest'
import { nextRunDisplay } from './nextRun'

describe('nextRunDisplay', () => {
  it('returns null for missing, zero and year 1 times', () => {
    expect(nextRunDisplay(undefined)).toBeNull()
    expect(nextRunDisplay(null)).toBeNull()
    expect(nextRunDisplay('0001-01-01T00:00:00Z')).toBeNull()
    expect(nextRunDisplay('not a date')).toBeNull()
  })

  it('formats a real future time', () => {
    expect(nextRunDisplay('2030-05-01T10:00:00Z')).toContain('2030')
  })
})
