import { describe, expect, it } from 'vitest'
import type { DiskSample } from '../../queries/diskHistory'
import {
  buildScale,
  distinctDays,
  nearestIndex,
  projection,
  rangeOptions,
  toPoints,
  usedBytes,
  xAt,
} from './trendModel'

const sample = (at: string, free: number): DiskSample => ({
  at,
  free_bytes: free,
  total_bytes: 100,
  images_bytes: 1,
  containers_bytes: 2,
  volumes_bytes: 3,
  build_cache_bytes: 4,
})

describe('trendModel', () => {
  it('derives used bytes and sorts samples', () => {
    const pts = toPoints([
      sample('2026-11-02T00:00:00Z', 50),
      sample('2026-11-01T00:00:00Z', 70),
    ])
    expect(pts.map((p) => p.used)).toEqual([30, 50])
    expect(usedBytes({ free_bytes: 200, total_bytes: 100 })).toBe(0)
    expect(distinctDays(pts)).toBe(2)
  })

  it('caps the range options by retention', () => {
    expect(rangeOptions(90)).toEqual([7, 30, 90])
    expect(rangeOptions(30)).toEqual([7, 30])
    expect(rangeOptions(3)).toEqual([3])
  })

  it('projects to total at the fill date, or not at all', () => {
    const pts = toPoints([
      sample('2026-11-01T00:00:00Z', 70),
      sample('2026-11-02T00:00:00Z', 60),
    ])
    expect(projection(pts, null)).toBeNull()
    const p = projection(pts, 10)
    expect(p?.to.used).toBe(100)
    const scale = buildScale(pts, p)
    expect(scale.yMax).toBe(100)
    expect(xAt(scale, scale.x1, 200)).toBe(200)
  })

  it('finds the nearest sample to a pixel', () => {
    const pts = toPoints([
      sample('2026-11-01T00:00:00Z', 70),
      sample('2026-11-02T00:00:00Z', 60),
      sample('2026-11-03T00:00:00Z', 50),
    ])
    const scale = buildScale(pts, null)
    expect(nearestIndex(pts, scale, 1, 300)).toBe(0)
    expect(nearestIndex(pts, scale, 299, 300)).toBe(2)
    expect(nearestIndex([], scale, 0, 300)).toBe(-1)
  })
})
