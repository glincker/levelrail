import { describe, expect, it } from 'vitest'
import type { ImageEntry } from '../../queries/imageInventory'
import type { StorageInventory } from '../../queries/storageInventory'
import {
  buildItemRows,
  filterRows,
  groupRows,
  largeUnmanagedBytes,
  parseView,
} from './itemRows'

function image(over: Partial<ImageEntry>): ImageEntry {
  return {
    id: 'sha256:abcdef1234567890',
    tags: [],
    size_bytes: 100,
    shared_bytes: 0,
    containers: 0,
    created_at: '2026-11-01T00:00:00Z',
    apps: [],
    protected: false,
    unmanaged: false,
    in_use: false,
    removable: true,
    ...over,
  }
}

const inventory: StorageInventory = {
  node: 'local',
  unsupported: false,
  volumes: [
    {
      name: 'db-main',
      size_bytes: 500,
      in_use: true,
      is_database: true,
      orphaned: false,
      protected: true,
      protect_reason: 'Holds the postgres data of shop',
      owner_app: 'shop',
    },
    {
      name: 'old-vol',
      in_use: false,
      is_database: false,
      orphaned: true,
      protected: false,
    },
  ],
  containers: [{ id: 'c1', name: 'web-1', size_bytes: 10, owner_app: 'shop' }],
}

describe('buildItemRows', () => {
  const rows = buildItemRows({
    images: [
      image({
        tags: ['shop:1'],
        in_use: true,
        apps: [{ app: 'shop', role: 'live' }],
      }),
      image({ id: 'sha256:big', unmanaged: true, size_bytes: 9000 }),
      image({ id: 'sha256:blk', blocked_reason: 'Used by a rollback target' }),
    ],
    inventory,
    buildCacheBytes: 70,
    largeBytes: 1000,
  })

  it('marks only exceptions amber', () => {
    const amber = rows.filter((r) => r.tone === 'amber').map((r) => r.name)
    expect(amber.sort()).toEqual(['big', 'blk', 'db-main'])
  })

  it('locks database volumes with the server reason', () => {
    const db = rows.find((r) => r.name === 'db-main')
    expect(db?.protected).toBe(true)
    expect(db?.reason.text).toBe('Holds the postgres data of shop')
  })

  it('explains orphaned volumes and unsized ones', () => {
    const old = rows.find((r) => r.name === 'old-vol')
    expect(old?.reason.code).toBe('orphaned')
    expect(old?.bytes).toBeNull()
    expect(old?.filter).toBe('other')
  })

  it('says which app uses an image', () => {
    expect(rows.find((r) => r.name === 'shop:1')?.reason).toEqual({
      code: 'inUseBy',
      app: 'shop',
    })
  })
})

describe('grouping', () => {
  const rows = buildItemRows({
    images: [
      image({
        tags: ['shop:1'],
        apps: [{ app: 'shop', role: 'live', at: '2026-11-02T00:00:00Z' }],
      }),
    ],
    inventory,
    buildCacheBytes: null,
    largeBytes: 1000,
  })

  it('groups by app with the unowned rows last', () => {
    const groups = groupRows(rows, 'app', 'size')
    expect(groups[0]?.app).toBe('shop')
    expect(groups[0]?.lastAt).toBe('2026-11-02T00:00:00Z')
    expect(groups[groups.length - 1]?.app).toBeNull()
  })

  it('groups by type in a fixed order and sorts by size', () => {
    const groups = groupRows(rows, 'type', 'size')
    expect(groups.map((g) => g.kind)).toEqual(['image', 'volume', 'container'])
    expect(groups[1]?.rows[0]?.name).toBe('db-main')
  })

  it('filters by category and search', () => {
    expect(filterRows(rows, 'containersLogs', '').map((r) => r.name)).toEqual([
      'web-1',
    ])
    expect(filterRows(rows, null, 'DB-').map((r) => r.name)).toEqual([
      'db-main',
    ])
  })

  it('treats unknown views as By type', () => {
    expect(parseView('nope')).toBe('type')
    expect(parseView('app')).toBe('app')
  })
})

describe('largeUnmanagedBytes', () => {
  const setting = (value: number) =>
    [{ key: 'storage.large_unmanaged_image_mb', value }] as never

  it('reads the runtime setting in MiB', () => {
    expect(largeUnmanagedBytes(setting(500))).toBe(500 * 1024 * 1024)
    expect(largeUnmanagedBytes(setting(1))).toBe(1024 * 1024)
  })

  it('flags nothing until the setting is known', () => {
    expect(largeUnmanagedBytes(undefined)).toBe(Number.POSITIVE_INFINITY)
    expect(largeUnmanagedBytes([])).toBe(Number.POSITIVE_INFINITY)
  })
})
