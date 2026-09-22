import { describe, expect, it, vi } from 'vitest'
import { ageInDays, formatAge } from './format'

describe('formatAge', () => {
  it('returns the fallback for an undefined timestamp', () => {
    expect(formatAge(undefined)).toBe('unknown')
    expect(formatAge(undefined, 'never set')).toBe('never set')
  })

  it('returns the fallback for an unparseable timestamp', () => {
    expect(formatAge('not-a-date')).toBe('unknown')
  })

  it('renders "just now" for a timestamp seconds ago', () => {
    const now = new Date('2026-09-20T12:00:00Z')
    vi.useFakeTimers()
    vi.setSystemTime(now)
    expect(formatAge('2026-09-20T11:59:50Z')).toBe('just now')
    vi.useRealTimers()
  })

  it('renders days ago for a multi-day-old timestamp', () => {
    const now = new Date('2026-09-20T12:00:00Z')
    vi.useFakeTimers()
    vi.setSystemTime(now)
    expect(formatAge('2026-09-16T12:00:00Z')).toBe('4 days ago')
    vi.useRealTimers()
  })

  it('renders months ago for a timestamp beyond 30 days', () => {
    const now = new Date('2026-09-20T12:00:00Z')
    vi.useFakeTimers()
    vi.setSystemTime(now)
    expect(formatAge('2026-06-20T12:00:00Z')).toBe('3 months ago')
    vi.useRealTimers()
  })
})

describe('ageInDays', () => {
  it('returns null for an undefined or unparseable timestamp', () => {
    expect(ageInDays(undefined)).toBeNull()
    expect(ageInDays('garbage')).toBeNull()
  })

  it('returns the whole number of elapsed days', () => {
    const now = new Date('2026-09-20T12:00:00Z')
    vi.useFakeTimers()
    vi.setSystemTime(now)
    expect(ageInDays('2026-06-22T12:00:00Z')).toBe(90)
    vi.useRealTimers()
  })
})
