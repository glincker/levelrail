import { describe, expect, it } from 'vitest'
import type { ImageInventory } from '../../queries/imageInventory'
import type { StorageInventory } from '../../queries/storageInventory'
import type { DockerDiskUsage } from '../../queries/systemStatus'
import type { RuntimeSetting } from '../../types/runtimeSettings'
import {
  buildDiskCategories,
  diskHealth,
  safeReclaimableBytes,
} from './storageData'
import { normalizeStorageSearch } from './storageTabs'

const MB = 1024 * 1024
const fallback = { lowPercent: 15, criticalPercent: 5 }

function setting(key: string, value: string): RuntimeSetting {
  return { key, value } as RuntimeSetting
}

const usage: DockerDiskUsage = {
  images_total_bytes: 9 * MB,
  images_reclaimable_bytes: 1 * MB,
  containers_total_bytes: 2 * MB,
  containers_reclaimable_bytes: 1 * MB,
  volumes_total_bytes: 3 * MB,
  volumes_reclaimable_bytes: 3 * MB,
  build_cache_total_bytes: 4 * MB,
  build_cache_reclaimable_bytes: 2 * MB,
}

const images = {
  images: [
    { id: 'sha256:aaa', tags: ['web:1'], size_bytes: 5 * MB },
    { id: 'sha256:bbb', tags: [], size_bytes: 3 * MB },
    { id: 'sha256:ccc', tags: ['db:1'], size_bytes: 2 * MB },
    { id: 'sha256:ddd', tags: ['x:1'], size_bytes: 1 * MB },
  ],
  totals: {
    size_bytes: 11 * MB,
    reclaimable_bytes: 2 * MB,
    unmanaged_unused_bytes: 1 * MB,
  },
} as unknown as ImageInventory

const inventory = {
  unsupported: false,
  volumes: [
    { name: 'a', size_bytes: 4 * MB, orphaned: true, protected: false },
    { name: 'db-main', size_bytes: 6 * MB, orphaned: true, is_database: true },
    { name: 'c', size_bytes: 1 * MB, orphaned: false },
  ],
  containers: [{ id: '1', name: 'old', size_bytes: MB }],
} as unknown as StorageInventory

describe('diskHealth', () => {
  const settings = [
    setting('disk.low_free_mb', '2048'),
    setting('disk.critical_free_mb', '512'),
  ]
  const run = (freeMb: number, s: RuntimeSetting[] | undefined) =>
    diskHealth({
      freeBytes: freeMb * MB,
      totalBytes: 100_000 * MB,
      settings: s,
      fallback,
    })

  it('uses the runtime floors', () => {
    expect(run(5000, settings)).toBe('healthy')
    expect(run(1000, settings)).toBe('low')
    expect(run(100, settings)).toBe('critical')
  })

  it('falls back to percent only without settings', () => {
    expect(run(50_000, undefined)).toBe('healthy')
    expect(run(10_000, undefined)).toBe('low')
    expect(run(1_000, undefined)).toBe('critical')
  })
})

describe('buildDiskCategories', () => {
  it('maps local data with top 3 items and a safe total that skips database volumes', () => {
    const c = buildDiskCategories({ usage, images, inventory, local: true })
    expect(c.images?.bytes).toBe(11 * MB)
    expect(c.images?.reclaimableBytes).toBe(3 * MB)
    expect(c.images?.topItems?.map((i) => i.name)).toEqual([
      'web:1',
      'bbb',
      'db:1',
    ])
    expect(c.appData?.bytes).toBe(11 * MB)
    expect(c.appData?.reclaimableBytes).toBe(4 * MB)
    expect(c.buildCache?.bytes).toBe(4 * MB)
    expect(c.containersLogs?.topItems?.[0]?.name).toBe('old')
    expect(safeReclaimableBytes(c)).toBe((3 + 4 + 2 + 1) * MB)
  })

  it('leaves volumes at zero reclaimable when only Docker totals exist', () => {
    const c = buildDiskCategories({
      usage,
      images: undefined,
      inventory: undefined,
      local: true,
    })
    expect(c.appData).toEqual({ bytes: 3 * MB, reclaimableBytes: 0 })
    expect(c.images?.bytes).toBe(9 * MB)
  })

  it('marks everything but images unmeasured on a remote node', () => {
    const c = buildDiskCategories({
      usage,
      images,
      inventory: undefined,
      local: false,
    })
    expect(c.appData).toEqual({ bytes: null })
    expect(c.buildCache).toEqual({ bytes: null })
    expect(c.containersLogs).toEqual({ bytes: null })
    expect(safeReclaimableBytes(c)).toBe(3 * MB)
  })

  it('marks categories unmeasured when Docker usage is missing', () => {
    const c = buildDiskCategories({
      usage: undefined,
      images: undefined,
      inventory: undefined,
      local: true,
    })
    expect(c.images).toEqual({ bytes: null })
    expect(safeReclaimableBytes(c)).toBe(0)
  })
})

describe('normalizeStorageSearch', () => {
  it('maps the cache tab to the build cache filter', () => {
    expect(normalizeStorageSearch({ tab: 'cache' })).toEqual({
      filter: 'buildCache',
    })
  })

  it('maps old image chips to the images filter and drops all', () => {
    expect(normalizeStorageSearch({ filter: 'unused' })).toEqual({
      filter: 'images',
    })
    expect(normalizeStorageSearch({ filter: 'all' })).toEqual({})
  })

  it('maps review to the cleanup drawer and keeps view and new filters', () => {
    expect(
      normalizeStorageSearch({ review: '1', view: 'app', filter: 'appData' }),
    ).toEqual({ filter: 'appData', view: 'app', drawer: 'cleanup' })
    expect(normalizeStorageSearch({ drawer: 'cleanup' })).toEqual({
      drawer: 'cleanup',
    })
  })
})
