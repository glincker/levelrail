import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearHistory,
  nextUp,
  normalizeKey,
  recordVisit,
  topFrecent,
} from './paletteHistory'

beforeEach(() => window.localStorage.clear())

const DAY = 24 * 60 * 60 * 1000

describe('paletteHistory', () => {
  it('strips group prefixes from keys', () => {
    expect(normalizeKey('suggested-quick-nav-backups')).toBe('nav-backups')
    expect(normalizeKey('recent-nav-apps')).toBe('nav-apps')
  })

  it('ranks by use, and a stale favourite loses to a fresh one', () => {
    const now = Date.now()
    recordVisit('nav-a', now - 60 * DAY)
    recordVisit('nav-a', now - 60 * DAY)
    recordVisit('nav-a', now - 60 * DAY)
    recordVisit('nav-b', now)
    recordVisit('nav-b', now)
    expect(topFrecent(2, now)).toEqual(['nav-b', 'nav-a'])
  })

  it('needs two opens before something counts as most used', () => {
    recordVisit('nav-once')
    expect(topFrecent(5)).toEqual([])
  })

  it('learns what follows what', () => {
    for (let i = 0; i < 2; i++) {
      recordVisit('nav-deploys')
      recordVisit('nav-logs')
    }
    recordVisit('nav-deploys')
    expect(nextUp(3)).toEqual(['nav-logs'])
  })

  it('clears everything', () => {
    recordVisit('nav-a')
    recordVisit('nav-a')
    clearHistory()
    expect(topFrecent(5)).toEqual([])
  })
})
