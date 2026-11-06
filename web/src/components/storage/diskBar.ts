export const DISK_CATEGORIES = [
  'appData',
  'images',
  'buildCache',
  'containersLogs',
  'other',
] as const

export type DiskCategory = (typeof DISK_CATEGORIES)[number]

export interface DiskTopItem {
  name: string
  bytes: number
}

export interface DiskCategoryInput {
  /** Null means not measured: shown as a dotted outline, never guessed. */
  bytes: number | null
  reclaimableBytes?: number
  topItems?: DiskTopItem[]
}

export interface DiskSegment {
  id: DiskCategory
  bytes: number | null
  reclaimableBytes: number
  topItems: DiskTopItem[]
  /** Share of the whole disk, 0 to 1. */
  share: number
}

export type DiskColumnId = DiskCategory | 'free'

/** Unknown sizes still need room on the bar so the dotted outline is visible. */
const UNMEASURED_SHARE = 0.04

export function buildDiskSegments(input: {
  totalBytes: number
  freeBytes: number
  categories: Partial<Record<DiskCategory, DiskCategoryInput>>
}): DiskSegment[] {
  const { totalBytes, freeBytes, categories } = input
  const given = DISK_CATEGORIES.filter(
    (id) => id !== 'other' && categories[id] !== undefined,
  )
  const unmeasured = given.some((id) => categories[id]?.bytes === null)
  const measured = given.reduce((n, id) => n + (categories[id]?.bytes ?? 0), 0)
  // With an unmeasured category the remainder cannot be told apart from it.
  const derivedOther =
    unmeasured || categories.other !== undefined
      ? null
      : Math.max(0, totalBytes - freeBytes - measured)

  return DISK_CATEGORIES.flatMap((id): DiskSegment[] => {
    const c = categories[id]
    const derived = id === 'other' && !c ? derivedOther : null
    if (!c && derived === null) return []
    const bytes = c ? c.bytes : derived
    return [
      {
        id,
        bytes,
        reclaimableBytes: Math.min(c?.reclaimableBytes ?? 0, bytes ?? 0),
        topItems: (c?.topItems ?? []).slice(0, 3),
        share: bytes !== null && totalBytes > 0 ? bytes / totalBytes : 0,
      },
    ]
  })
}

/** Weights for the grid columns: bytes for measured, a fixed share for unknown. */
export function columnWeights(
  segments: DiskSegment[],
  totalBytes: number,
  freeBytes: number,
): { id: DiskColumnId; weight: number }[] {
  const base = Math.max(totalBytes, 1)
  const cols: { id: DiskColumnId; weight: number }[] = segments
    .map((s) => ({
      id: s.id,
      weight: s.bytes === null ? base * UNMEASURED_SHARE : s.bytes,
    }))
    .filter((c) => c.weight > 0)
  if (freeBytes > 0) cols.push({ id: 'free', weight: freeBytes })
  return cols
}

/** Row resolution: segments snap to 0.5 percent, so the row is 200 units. */
export const ROW_UNITS = 200

/** Quantise weights to whole units (min 1) summing to ROW_UNITS; the biggest absorbs rounding. */
export function quantizeUnits(weights: number[]): number[] {
  const sum = weights.reduce((n, w) => n + w, 0) || 1
  const units = weights.map((w) =>
    Math.max(1, Math.round((w / sum) * ROW_UNITS)),
  )
  if (units.length === 0) return units
  let big = 0
  units.forEach((u, i) => {
    if (u > (units[big] ?? 0)) big = i
  })
  const diff = ROW_UNITS - units.reduce((n, u) => n + u, 0)
  units[big] = Math.max(1, (units[big] ?? 0) + diff)
  return units
}

/** Reclaimable part of a segment in units of 0.5 percent of that segment. */
export function hatchUnits(bytes: number, reclaimable: number): number {
  if (bytes <= 0 || reclaimable <= 0) return 0
  const r = Math.min(reclaimable, bytes)
  return Math.min(ROW_UNITS, Math.max(1, Math.round((r / bytes) * ROW_UNITS)))
}
