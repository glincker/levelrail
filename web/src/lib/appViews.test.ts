import { beforeEach, describe, expect, it } from 'vitest'
import {
  EMPTY_FILTERS,
  filtersActive,
  loadSavedViews,
  storeSavedViews,
} from './appViews'

describe('saved app views', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('round-trips views through storage', () => {
    const views = [
      {
        name: 'prod edge',
        filters: {
          tags: ['tier:edge'],
          environments: ['production'],
          query: '',
        },
      },
    ]
    storeSavedViews(views)
    expect(loadSavedViews()).toEqual(views)
  })

  it.each([['not json'], ['{"a":1}'], ['[{"name":1}]'], ['[null]']])(
    'ignores malformed storage %s',
    (raw) => {
      window.localStorage.setItem('apps.savedViews.v1', raw)
      expect(loadSavedViews()).toEqual([])
    },
  )

  it('reports whether any filter is set', () => {
    expect(filtersActive(EMPTY_FILTERS)).toBe(false)
    expect(filtersActive({ ...EMPTY_FILTERS, query: 'web' })).toBe(true)
    expect(filtersActive({ ...EMPTY_FILTERS, tags: ['a'] })).toBe(true)
  })
})
