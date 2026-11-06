import type { ImageInventory } from '../../queries/imageInventory'
import type { StorageInventory } from '../../queries/storageInventory'
import type { DockerDiskUsage } from '../../queries/systemStatus'
import type { RuntimeSetting } from '../../types/runtimeSettings'
import { imageTitle, shortId } from '../images/imageFormat'
import type { DiskCategory, DiskCategoryInput, DiskTopItem } from './diskBar'
import { POLICY_KEYS, settingNumber } from './policyChips'
import type { DiskHealth } from './StorageVerdict'

const MB = 1024 * 1024
const TOP = 3

export interface PercentFloors {
  lowPercent: number
  criticalPercent: number
}

function top(items: DiskTopItem[]): DiskTopItem[] {
  return items
    .filter((i) => i.bytes > 0)
    .sort((a, b) => b.bytes - a.bytes)
    .slice(0, TOP)
}

/**
 * Health from the runtime disk floors; percent floors apply only when the
 * settings could not be read, so a missing setting never reads as healthy by luck.
 */
export function diskHealth(input: {
  freeBytes: number
  totalBytes: number
  settings: RuntimeSetting[] | undefined
  fallback: PercentFloors
}): DiskHealth {
  const { freeBytes, totalBytes, settings, fallback } = input
  const lowMb = settingNumber(settings, POLICY_KEYS.lowFreeMb)
  const criticalMb = settingNumber(settings, POLICY_KEYS.criticalFreeMb)
  if (lowMb !== null && criticalMb !== null) {
    if (freeBytes < criticalMb * MB) return 'critical'
    if (freeBytes < lowMb * MB) return 'low'
    return 'healthy'
  }
  if (totalBytes <= 0) return 'healthy'
  const pct = (freeBytes / totalBytes) * 100
  if (pct < fallback.criticalPercent) return 'critical'
  if (pct < fallback.lowPercent) return 'low'
  return 'healthy'
}

function imageCategory(
  images: ImageInventory | undefined,
  usage: DockerDiskUsage | undefined,
): DiskCategoryInput {
  if (images) {
    const t = images.totals
    return {
      bytes: t.size_bytes,
      reclaimableBytes: t.reclaimable_bytes + t.unmanaged_unused_bytes,
      topItems: top(
        images.images.map((e) => ({
          name: imageTitle(e) ?? shortId(e.id),
          bytes: e.size_bytes,
        })),
      ),
    }
  }
  if (!usage) return { bytes: null }
  return {
    bytes: usage.images_total_bytes,
    reclaimableBytes: usage.images_reclaimable_bytes,
  }
}

function volumeCategory(
  inv: StorageInventory | undefined,
  usage: DockerDiskUsage | undefined,
): DiskCategoryInput {
  const vols =
    inv && !inv.unsupported
      ? inv.volumes.filter((v) => typeof v.size_bytes === 'number')
      : null
  if (vols && vols.length > 0) {
    // Only orphaned, non-database volumes count as safe; Docker's "unused" may include them.
    const safe = vols
      .filter((v) => v.orphaned && !v.protected && !v.is_database)
      .reduce((n, v) => n + (v.size_bytes ?? 0), 0)
    return {
      bytes: vols.reduce((n, v) => n + (v.size_bytes ?? 0), 0),
      reclaimableBytes: safe,
      topItems: top(
        vols.map((v) => ({ name: v.name, bytes: v.size_bytes ?? 0 })),
      ),
    }
  }
  if (!usage) return { bytes: null }
  return { bytes: usage.volumes_total_bytes, reclaimableBytes: 0 }
}

function containerCategory(
  inv: StorageInventory | undefined,
  usage: DockerDiskUsage | undefined,
): DiskCategoryInput {
  if (!usage) return { bytes: null }
  const named =
    inv && !inv.unsupported
      ? top(
          inv.containers.map((c) => ({
            name: c.name,
            bytes: c.size_bytes ?? 0,
          })),
        )
      : []
  return {
    bytes: usage.containers_total_bytes,
    reclaimableBytes: usage.containers_reclaimable_bytes,
    topItems: named,
  }
}

export interface StorageDataInput {
  usage: DockerDiskUsage | undefined
  images: ImageInventory | undefined
  inventory: StorageInventory | undefined
  /** Remote nodes only size images, so every other category stays unmeasured. */
  local: boolean
}

export function buildDiskCategories(
  input: StorageDataInput,
): Partial<Record<DiskCategory, DiskCategoryInput>> {
  const { usage, images, inventory, local } = input
  const docker = local ? usage : undefined
  return {
    appData: local ? volumeCategory(inventory, docker) : { bytes: null },
    images: imageCategory(images, docker),
    buildCache: docker
      ? {
          bytes: docker.build_cache_total_bytes,
          reclaimableBytes: docker.build_cache_reclaimable_bytes,
        }
      : { bytes: null },
    containersLogs: local
      ? containerCategory(inventory, docker)
      : { bytes: null },
  }
}

/** Sum of what each measured category marks safe; unmeasured ones add nothing. */
export function safeReclaimableBytes(
  categories: Partial<Record<DiskCategory, DiskCategoryInput>>,
): number {
  return Object.values(categories).reduce(
    (n, c) => n + (c && c.bytes !== null ? (c.reclaimableBytes ?? 0) : 0),
    0,
  )
}
