export type SnoozeDuration = '1h' | '1d' | 'forever'

// Epoch ms the snooze ends at; null means forever.
export type SnoozeMap = Record<string, number | null>

const HOUR_MS = 60 * 60 * 1000

export const SNOOZE_LABELS: Record<SnoozeDuration, string> = {
  '1h': '1 hour',
  '1d': '1 day',
  forever: 'Forever',
}

export function snoozeUntil(now: number, d: SnoozeDuration): number | null {
  if (d === 'forever') return null
  return now + (d === '1h' ? HOUR_MS : 24 * HOUR_MS)
}

export function snoozeIssue(
  map: SnoozeMap,
  id: string,
  d: SnoozeDuration,
  now: number,
): SnoozeMap {
  return { ...map, [id]: snoozeUntil(now, d) }
}

export function unsnoozeIssue(map: SnoozeMap, id: string): SnoozeMap {
  return Object.fromEntries(Object.entries(map).filter(([k]) => k !== id))
}

export function isSnoozed(map: SnoozeMap, id: string, now: number): boolean {
  if (!(id in map)) return false
  const until = map[id]
  if (until === null || until === undefined) return true
  return now < until
}

export function pruneExpired(map: SnoozeMap, now: number): SnoozeMap {
  return Object.fromEntries(
    Object.entries(map).filter(([, until]) => until === null || now < until),
  )
}

export function isSnoozeMap(v: unknown): v is SnoozeMap {
  return (
    typeof v === 'object' &&
    v !== null &&
    !Array.isArray(v) &&
    Object.values(v).every((x) => x === null || typeof x === 'number')
  )
}
