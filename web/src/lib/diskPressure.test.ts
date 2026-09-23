import { describe, expect, it } from 'vitest'
import { assessDiskPressure } from './diskPressure'

const GB = 1024 ** 3
const usage = {
  images_total_bytes: 0,
  images_reclaimable_bytes: 2 * GB,
  containers_total_bytes: 0,
  containers_reclaimable_bytes: GB,
  volumes_total_bytes: 0,
  volumes_reclaimable_bytes: 0,
  build_cache_total_bytes: 0,
  build_cache_reclaimable_bytes: 3 * GB,
}

describe('assessDiskPressure', () => {
  it.each([
    { name: 'plenty of space', free: 50, level: 'ok' },
    { name: 'below warn', free: 10, level: 'warning' },
    { name: 'below critical', free: 3, level: 'critical' },
  ])('$name', ({ free, level }) => {
    const got = assessDiskPressure({
      data_dir_total_bytes: 100 * GB,
      data_dir_free_bytes: free * GB,
      docker_disk_usage: usage,
    })
    expect(got?.level).toBe(level)
    expect(got?.reclaimableBytes).toBe(6 * GB)
  })

  it('returns undefined with no disk data', () => {
    expect(assessDiskPressure({})).toBeUndefined()
  })
})
