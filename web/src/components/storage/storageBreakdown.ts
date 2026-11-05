import type { DockerDiskUsage } from '../../queries/systemStatus'

export const STORAGE_CATEGORIES = [
  'images',
  'containers',
  'volumes',
  'buildCache',
] as const

export type StorageCategory = (typeof STORAGE_CATEGORIES)[number]

export interface StorageSegment {
  id: StorageCategory
  /** Null means no API sizes this category for the node yet. */
  totalBytes: number | null
  reclaimableBytes: number
}

export interface ImageTotalsInput {
  sizeBytes: number
  reclaimableBytes: number
}

/**
 * Local nodes read Docker's own accounting; the image inventory is the only
 * per-node source, so every other category on a remote node stays unmeasured.
 */
export function buildSegments(input: {
  usage: DockerDiskUsage | undefined
  images: ImageTotalsInput | undefined
  local: boolean
}): StorageSegment[] {
  const { usage, images, local } = input
  const docker = local ? usage : undefined
  return [
    {
      id: 'images',
      totalBytes: images?.sizeBytes ?? docker?.images_total_bytes ?? null,
      reclaimableBytes:
        images?.reclaimableBytes ?? docker?.images_reclaimable_bytes ?? 0,
    },
    {
      id: 'containers',
      totalBytes: docker?.containers_total_bytes ?? null,
      reclaimableBytes: docker?.containers_reclaimable_bytes ?? 0,
    },
    {
      id: 'volumes',
      totalBytes: docker?.volumes_total_bytes ?? null,
      reclaimableBytes: docker?.volumes_reclaimable_bytes ?? 0,
    },
    {
      id: 'buildCache',
      totalBytes: docker?.build_cache_total_bytes ?? null,
      reclaimableBytes: docker?.build_cache_reclaimable_bytes ?? 0,
    },
  ]
}

/** Bar widths come from fixed steps because Tailwind needs literal class names. */
export const WIDTH_STEPS = [
  'w-[0%]',
  'w-[5%]',
  'w-[10%]',
  'w-[15%]',
  'w-[20%]',
  'w-[25%]',
  'w-[30%]',
  'w-[35%]',
  'w-[40%]',
  'w-[45%]',
  'w-[50%]',
  'w-[55%]',
  'w-[60%]',
  'w-[65%]',
  'w-[70%]',
  'w-[75%]',
  'w-[80%]',
  'w-[85%]',
  'w-[90%]',
  'w-[95%]',
  'w-[100%]',
] as const

/** Share of the measured total as a width step, never 0 for a non-empty segment. */
export function widthStep(bytes: number, sum: number): string {
  if (bytes <= 0 || sum <= 0) return WIDTH_STEPS[0]
  const step = Math.max(1, Math.round((bytes / sum) * 20))
  return WIDTH_STEPS[Math.min(step, 20)] ?? WIDTH_STEPS[20]
}
