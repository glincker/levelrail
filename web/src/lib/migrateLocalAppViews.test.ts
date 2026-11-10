import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadSavedViews, storeSavedViews } from './appViews'
import { migrateLocalAppViews } from './migrateLocalAppViews'

const view = (name: string, query: string) => ({
  name,
  filters: { query, tags: ['web'], environments: ['prod', 'stage'] },
})

beforeEach(() => window.localStorage.clear())
afterEach(() => window.localStorage.clear())

describe('migrateLocalAppViews', () => {
  it('sends each view and clears the browser copy once all are accepted', async () => {
    storeSavedViews([view('api', 'api'), view('web', '')])
    const save = vi.fn(() => Promise.resolve({}))
    expect(await migrateLocalAppViews(new Map(), save)).toBe(2)
    expect(save).toHaveBeenCalledWith({
      name: 'api',
      filter: { q: 'api', env: 'prod,stage', tags: 'web' },
    })
    expect(window.localStorage.getItem('apps.savedViews.v1')).toBeNull()
  })

  it('keeps views the server refused or that clash with another filter', async () => {
    storeSavedViews([view('ok', 'a'), view('bad', 'b'), view('clash', 'c')])
    const save = vi.fn((d: { name: string }) =>
      d.name === 'bad' ? Promise.reject(new Error('400')) : Promise.resolve({}),
    )
    const existing = new Map([['clash', { q: 'other' }]])
    expect(await migrateLocalAppViews(existing, save)).toBe(1)
    expect(loadSavedViews().map((v) => v.name)).toEqual(['bad', 'clash'])
  })

  it('treats a same named server view with the same filter as moved', async () => {
    storeSavedViews([view('api', 'api')])
    const save = vi.fn(() => Promise.resolve({}))
    const existing = new Map([
      ['api', { q: 'api', env: 'prod,stage', tags: 'web' }],
    ])
    expect(await migrateLocalAppViews(existing, save)).toBe(1)
    expect(save).not.toHaveBeenCalled()
    expect(loadSavedViews()).toEqual([])
  })
})
