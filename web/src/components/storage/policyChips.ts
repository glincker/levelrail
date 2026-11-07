import type { JanitorPolicy } from '@/queries/janitor'
import type { RuntimeSetting } from '@/types/runtimeSettings'

export const POLICY_KEYS = {
  keepPerApp: 'images.keep_per_app',
  lowFreeMb: 'disk.low_free_mb',
  criticalFreeMb: 'disk.critical_free_mb',
  lowFreePercent: 'storage.low_free_percent',
  criticalFreePercent: 'storage.critical_free_percent',
} as const

const MB_PER_GB = 1024

export type CleanupState = 'off' | 'dryRun' | 'on'

export type ChipId =
  'cleanup' | 'keepImages' | 'cacheAge' | 'lowDisk' | 'criticalDisk'

export interface PolicyChip {
  id: ChipId
  /** Current value in the editor's unit; null while unknown. */
  value: number | null
  unit: 'images' | 'hours' | 'gb' | 'percent'
}

export function settingNumber(
  settings: RuntimeSetting[] | undefined,
  key: string,
): number | null {
  const found = settings?.find((s) => s.key === key)
  if (!found) return null
  const n = Number(found.value)
  return Number.isFinite(n) ? n : null
}

export function mbToGb(mb: number): number {
  return Math.round((mb / MB_PER_GB) * 10) / 10
}

export function gbToMb(gb: number): number {
  return Math.round(gb * MB_PER_GB)
}

export function cleanupState(policy: JanitorPolicy): CleanupState {
  if (!policy.enabled) return 'off'
  return policy.dry_run ? 'dryRun' : 'on'
}

/** Whole days when the age divides evenly, hours otherwise. */
export function ageParts(hours: number): { unit: 'days' | 'hours'; n: number } {
  return hours >= 24 && hours % 24 === 0
    ? { unit: 'days', n: hours / 24 }
    : { unit: 'hours', n: hours }
}

/** Parses editor text; null means invalid. The server owns the real range. */
export function parsePositive(text: string, whole: boolean): number | null {
  const trimmed = text.trim()
  if (trimmed === '') return null
  const n = Number(trimmed)
  if (!Number.isFinite(n) || n <= 0) return null
  if (whole && !Number.isInteger(n)) return null
  return n
}

export function buildChips(
  janitor: JanitorPolicy | null,
  settings: RuntimeSetting[] | undefined,
): PolicyChip[] {
  const low = settingNumber(settings, POLICY_KEYS.lowFreeMb)
  const critical = settingNumber(settings, POLICY_KEYS.criticalFreeMb)
  const chips: PolicyChip[] = []
  if (janitor) {
    chips.push({
      id: 'cleanup',
      value: janitor.disk_free_budget_percent,
      unit: 'percent',
    })
  }
  chips.push({
    id: 'keepImages',
    value: settingNumber(settings, POLICY_KEYS.keepPerApp),
    unit: 'images',
  })
  if (janitor) {
    chips.push({
      id: 'cacheAge',
      value: janitor.build_cache_max_age_hours,
      unit: 'hours',
    })
  }
  chips.push({
    id: 'lowDisk',
    value: low === null ? null : mbToGb(low),
    unit: 'gb',
  })
  chips.push({
    id: 'criticalDisk',
    value: critical === null ? null : mbToGb(critical),
    unit: 'gb',
  })
  return chips
}
