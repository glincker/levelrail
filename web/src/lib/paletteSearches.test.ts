import { describe, expect, it } from 'vitest'
import { isMeaningfulSearch, withRecentSearch } from './paletteSearches'

describe('recent searches', () => {
  it('ignores typing fragments and punctuation', () => {
    expect(isMeaningfulSearch('re')).toBe(false)
    expect(isMeaningfulSearch('a;')).toBe(false)
    expect(isMeaningfulSearch('  ')).toBe(false)
    expect(isMeaningfulSearch('backup')).toBe(true)
  })

  it('lets a longer search replace the one it extends', () => {
    let list = withRecentSearch([], 'cle')
    list = withRecentSearch(list, 'clean')
    list = withRecentSearch(list, 'cleanup center')
    expect(list).toEqual(['cleanup center'])
  })

  it('does not keep a fragment of a search it already has', () => {
    expect(withRecentSearch(['cleanup center'], 'cleanup')).toEqual([
      'cleanup center',
    ])
  })

  it('moves a repeated search to the front', () => {
    expect(withRecentSearch(['backups', 'firewall'], 'firewall')).toEqual([
      'firewall',
      'backups',
    ])
  })
})
