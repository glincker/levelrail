const UNIT_MS: Record<string, number> = {
  h: 3_600_000,
  m: 60_000,
  s: 1000,
}

/** Reads a Go style duration like 1h30m into milliseconds, or 0 when it is not one. */
export function parseGoDuration(value: unknown): number {
  if (typeof value !== 'string') return 0
  let total = 0
  for (const m of value.matchAll(/(\d+(?:\.\d+)?)([hms])/g)) {
    total += Number(m[1]) * (UNIT_MS[m[2] ?? ''] ?? 0)
  }
  return total
}

const DAY_MS = 86_400_000
export const EXPIRY_MS: Record<string, number> = {
  '1d': DAY_MS,
  '7d': 7 * DAY_MS,
  '30d': 30 * DAY_MS,
  never: 0,
}

/** An expiry choice is allowed when it never expires or is at least the minimum lifetime. */
export function expiryAllowed(choice: string, minTtlMs: number): boolean {
  const ms = EXPIRY_MS[choice] ?? 0
  return ms === 0 || ms >= minTtlMs
}

// Branch states from which each action is accepted.
export const ACTION_STATES: Record<'reset' | 'promote' | 'delete', string[]> = {
  reset: ['ready', 'failed'],
  promote: ['ready'],
  delete: ['ready', 'failed'],
}
