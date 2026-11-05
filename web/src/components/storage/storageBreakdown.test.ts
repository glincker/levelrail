import { describe, expect, it } from 'vitest'
import type { DockerDiskUsage } from '../../queries/systemStatus'
import { buildSegments, widthStep, WIDTH_STEPS } from './storageBreakdown'

const usage: DockerDiskUsage = {
  images_total_bytes: 100,
  images_reclaimable_bytes: 10,
  containers_total_bytes: 20,
  containers_reclaimable_bytes: 2,
  volumes_total_bytes: 30,
  volumes_reclaimable_bytes: 0,
  build_cache_total_bytes: 40,
  build_cache_reclaimable_bytes: 4,
}

describe('buildSegments', () => {
  it('keeps docker system df order and uses Docker numbers locally', () => {
    const s = buildSegments({ usage, images: undefined, local: true })
    expect(s.map((x) => x.id)).toEqual([
      'images',
      'containers',
      'volumes',
      'buildCache',
    ])
    expect(s.map((x) => x.totalBytes)).toEqual([100, 20, 30, 40])
    expect(s[3]?.reclaimableBytes).toBe(4)
  })

  it('prefers the image inventory for images', () => {
    const s = buildSegments({
      usage,
      images: { sizeBytes: 7, reclaimableBytes: 3 },
      local: true,
    })
    expect(s[0]).toMatchObject({ totalBytes: 7, reclaimableBytes: 3 })
  })

  it('leaves unsized categories null, never zero', () => {
    const s = buildSegments({
      usage,
      images: { sizeBytes: 7, reclaimableBytes: 3 },
      local: false,
    })
    expect(s.map((x) => x.totalBytes)).toEqual([7, null, null, null])
    const none = buildSegments({
      usage: undefined,
      images: undefined,
      local: true,
    })
    expect(none.every((x) => x.totalBytes === null)).toBe(true)
  })
})

describe('widthStep', () => {
  it('gives a small non-empty segment a visible step', () => {
    expect(widthStep(1, 1000)).toBe(WIDTH_STEPS[1])
    expect(widthStep(0, 1000)).toBe(WIDTH_STEPS[0])
    expect(widthStep(1000, 1000)).toBe(WIDTH_STEPS[20])
  })
})
