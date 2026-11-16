const HOUR_MS = 3_600_000
const DAY_MS = 24 * HOUR_MS
const HOURS_THRESHOLD = 48

export type ExpiryCountdown =
  | { kind: 'none' }
  | { kind: 'expired'; days: number }
  | { kind: 'hours' | 'days'; count: number }

// Days normally, hours once under 48h so "expires in 1 day" never hides a short fuse.
export function expiryCountdown(
  expiresAt: string | undefined,
  now: number,
): ExpiryCountdown {
  if (!expiresAt) return { kind: 'none' }
  const at = new Date(expiresAt).getTime()
  if (Number.isNaN(at)) return { kind: 'none' }
  const diff = at - now
  if (diff < 0) return { kind: 'expired', days: Math.ceil(-diff / DAY_MS) }
  if (diff < HOURS_THRESHOLD * HOUR_MS) {
    return { kind: 'hours', count: Math.max(1, Math.ceil(diff / HOUR_MS)) }
  }
  return { kind: 'days', count: Math.ceil(diff / DAY_MS) }
}

// YYYY-MM-DD in UTC, which is what the rotate endpoint parses.
export function defaultRotateExpiry(lifetimeDays: number, now: number): string {
  return new Date(now + lifetimeDays * DAY_MS).toISOString().slice(0, 10)
}

export const ROTATE_DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/
