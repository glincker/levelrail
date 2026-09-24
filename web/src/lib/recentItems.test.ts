import { afterEach, describe, expect, it, vi } from 'vitest'
import { loadRecentKeys, pushRecentKey, withRecent } from './recentItems'

afterEach(() => {
  window.localStorage.clear()
  vi.restoreAllMocks()
})

describe('withRecent', () => {
  const cases: Array<[string[], string, string[]]> = [
    [[], 'a', ['a']],
    [['a', 'b'], 'c', ['c', 'a', 'b']],
    [['a', 'b', 'c'], 'c', ['c', 'a', 'b']],
    [['a', 'b', 'c', 'd', 'e'], 'f', ['f', 'a', 'b', 'c', 'd']],
  ]
  it.each(cases)('%j + %s', (keys, key, want) => {
    expect(withRecent(keys, key)).toEqual(want)
  })
})

describe('recent persistence', () => {
  it('round-trips through localStorage', () => {
    pushRecentKey('nav-status')
    pushRecentKey('nav-apps')
    expect(loadRecentKeys()).toEqual(['nav-apps', 'nav-status'])
  })

  it('ignores corrupt stored data', () => {
    window.localStorage.setItem('palette.recent', '{not json')
    expect(loadRecentKeys()).toEqual([])
    window.localStorage.setItem('palette.recent', '{"a":1}')
    expect(loadRecentKeys()).toEqual([])
  })

  it('works when storage throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(loadRecentKeys()).toEqual([])
    expect(pushRecentKey('x')).toEqual(['x'])
  })
})
