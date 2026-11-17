import { describe, expect, it } from 'vitest'
import { alertsPageState } from './alertsPageState'

const none = {
  loading: false,
  unread: 0,
  hasInbox: false,
  hasHistory: false,
  hasSilences: false,
  hasWindows: false,
}

describe('alertsPageState', () => {
  it('is loading while any read is pending', () => {
    expect(alertsPageState({ ...none, loading: true, unread: 3 })).toBe(
      'loading',
    )
  })
  it('is empty with nothing configured or received', () => {
    expect(alertsPageState(none)).toBe('empty')
  })
  it('is idle when history or config exists but nothing is unread', () => {
    expect(alertsPageState({ ...none, hasHistory: true })).toBe('idle')
    expect(alertsPageState({ ...none, hasSilences: true })).toBe('idle')
    expect(alertsPageState({ ...none, hasWindows: true })).toBe('idle')
    expect(alertsPageState({ ...none, hasInbox: true })).toBe('idle')
  })
  it('is active when something is unread', () => {
    expect(alertsPageState({ ...none, hasInbox: true, unread: 2 })).toBe(
      'active',
    )
  })
})
