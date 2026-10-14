import { describe, expect, it } from 'vitest'
import { appScopeName, databaseScopeName } from './sidebarScope'

describe('sidebar scope', () => {
  it.each([
    ['/apps/shop', 'shop'],
    ['/apps/shop/domains', 'shop'],
    ['/apps/new', undefined],
    ['/apps/new/', undefined],
    ['/apps', undefined],
    ['/settings/apps/shop', undefined],
  ])('app scope of %s is %s', (path, want) => {
    expect(appScopeName(path)).toBe(want)
  })

  it.each([
    ['/databases/main', 'main'],
    ['/databases/main/backups', 'main'],
    ['/databases/new', undefined],
    ['/databases', undefined],
  ])('database scope of %s is %s', (path, want) => {
    expect(databaseScopeName(path)).toBe(want)
  })
})
