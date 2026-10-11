export type AgeUnit = 'seconds' | 'minutes' | 'hours' | 'days'

export interface Age {
  unit: AgeUnit
  count: number
}

/** Splits an elapsed time into the largest whole unit, clamped at zero. */
export function ageOf(from: string | Date, now: number): Age | null {
  const ms = from instanceof Date ? from.getTime() : Date.parse(from)
  if (!Number.isFinite(ms)) return null
  const secs = Math.max(0, Math.round((now - ms) / 1000))
  if (secs < 60) return { unit: 'seconds', count: secs }
  if (secs < 3600) return { unit: 'minutes', count: Math.floor(secs / 60) }
  if (secs < 86400) return { unit: 'hours', count: Math.floor(secs / 3600) }
  return { unit: 'days', count: Math.floor(secs / 86400) }
}

const DAY_MS = 86_400_000

/** Whole days until `notAfter`, negative once past; null if unparsable. */
export function daysUntil(notAfter: string, now: number): number | null {
  const end = Date.parse(notAfter)
  if (!Number.isFinite(end)) return null
  return Math.floor((end - now) / DAY_MS)
}
