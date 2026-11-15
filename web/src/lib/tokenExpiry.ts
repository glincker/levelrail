const HOUR_MS = 60 * 60 * 1000
const DAY_MS = 24 * HOUR_MS
// Under this many hours the remaining time reads in hours, not days.
const HOURS_ONLY_BELOW = 48

export type TokenExpiryState = 'none' | 'ok' | 'soon' | 'expired'

export interface TokenExpiry {
  state: TokenExpiryState
  unit: 'days' | 'hours'
  count: number
}

// Fallback mirrors the server default window (credential_expiry.reminder_windows_days).
export const DEFAULT_SOON_DAYS = 14

// The largest reminder window decides "expires soon". Setting value is a
// list like ["14","7","1"] or the raw "14:7:1" string.
export function soonDaysFromSetting(value: unknown): number {
  const parts = Array.isArray(value)
    ? value.map(String)
    : typeof value === 'string'
      ? value.split(/[:,\s]+/)
      : []
  const days = parts.map((p) => Number.parseInt(p, 10)).filter((n) => n > 0)
  return days.length > 0 ? Math.max(...days) : DEFAULT_SOON_DAYS
}

export function tokenExpiry(
  expiresAt: string | undefined,
  now: number,
  soonDays: number,
): TokenExpiry {
  if (!expiresAt) return { state: 'none', unit: 'days', count: 0 }
  const left = Date.parse(expiresAt) - now
  if (Number.isNaN(left)) return { state: 'none', unit: 'days', count: 0 }
  if (left <= 0) return { state: 'expired', unit: 'hours', count: 0 }
  const hours = Math.max(1, Math.ceil(left / HOUR_MS))
  const state = left <= soonDays * DAY_MS ? 'soon' : 'ok'
  if (hours < HOURS_ONLY_BELOW) return { state, unit: 'hours', count: hours }
  return { state, unit: 'days', count: Math.ceil(left / DAY_MS) }
}

// Whole hours left in a grace period, never below one while it still runs.
export function hoursUntil(at: string | undefined, now: number): number {
  if (!at) return 0
  const left = Date.parse(at) - now
  return left > 0 ? Math.max(1, Math.ceil(left / HOUR_MS)) : 0
}
