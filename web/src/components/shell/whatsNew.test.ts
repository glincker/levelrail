import { describe, expect, it } from 'vitest'
import type { ChangelogEntry } from '../../queries/changelog'
import { unreadCount } from './whatsNew'

const entries: ChangelogEntry[] = [
  { version: '0.3.0', date: '2026-10-01', bullets: ['c'] },
  { version: '0.2.0', date: '2026-09-01', bullets: ['b'] },
  { version: '0.1.0', date: '2026-08-01', bullets: ['a'] },
]

describe('unreadCount', () => {
  it('counts every entry when never seen', () => {
    expect(unreadCount(entries, '')).toBe(3)
  })

  it('counts zero once the latest version was seen', () => {
    expect(unreadCount(entries, '0.3.0')).toBe(0)
  })

  it('counts only entries newer than the seen version', () => {
    expect(unreadCount(entries, '0.2.0')).toBe(1)
  })

  it('counts everything when the seen version is unknown', () => {
    expect(unreadCount(entries, '0.0.1')).toBe(3)
  })

  it('returns zero for an empty entry list', () => {
    expect(unreadCount([], '')).toBe(0)
  })
})
