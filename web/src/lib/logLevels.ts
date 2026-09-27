import type { LogEntry } from '../types/logs'

export type LevelBucket = 'error' | 'warn' | 'info' | 'debug' | 'none'

export const LEVEL_BUCKETS: readonly LevelBucket[] = [
  'error',
  'warn',
  'info',
  'debug',
  'none',
]

// fatal counts as error and trace as debug, so the four named chips cover
// every level the API can return.
export function bucketOf(level: string | undefined): LevelBucket {
  switch (level) {
    case 'fatal':
    case 'error':
      return 'error'
    case 'warn':
      return 'warn'
    case 'info':
      return 'info'
    case 'trace':
    case 'debug':
      return 'debug'
    default:
      return 'none'
  }
}

export type LevelCounts = Record<LevelBucket, number>

export function countLevels(entries: readonly LogEntry[]): LevelCounts {
  const counts: LevelCounts = { error: 0, warn: 0, info: 0, debug: 0, none: 0 }
  for (const entry of entries) {
    counts[bucketOf(entry.level)] += 1
  }
  return counts
}

// An empty selection means no filter.
export function filterByLevels(
  entries: readonly LogEntry[],
  selected: ReadonlySet<LevelBucket>,
): LogEntry[] {
  if (selected.size === 0) {
    return [...entries]
  }
  return entries.filter((entry) => selected.has(bucketOf(entry.level)))
}

export function toggleLevel(
  selected: ReadonlySet<LevelBucket>,
  bucket: LevelBucket,
): Set<LevelBucket> {
  const next = new Set(selected)
  if (next.has(bucket)) {
    next.delete(bucket)
  } else {
    next.add(bucket)
  }
  return next
}

// "Showing N of total" line. total is the API's count for the range and
// query; loaded is what the page fetched (capped by the request limit).
export function summarizeShown(
  shown: number,
  loaded: number,
  total: number,
  levelFilterActive: boolean,
): string {
  const n = (v: number) => v.toLocaleString()
  if (levelFilterActive) {
    const base = `Showing ${n(shown)} of ${n(loaded)} loaded`
    return total > loaded ? `${base} (${n(total)} in range)` : base
  }
  return `Showing ${n(shown)} of ${n(Math.max(total, loaded))}`
}
