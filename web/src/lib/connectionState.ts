import { ApiError } from './apiError'

export const BACKOFF_BASE_MS = 2000
export const BACKOFF_MAX_MS = 30000
export const OFFLINE_FAILURE_THRESHOLD = 2
export const OFFLINE_FAILURE_WINDOW_MS = 10000

const GATEWAY_STATUSES = new Set([502, 503, 504])

/** Delay before probe number `attempt` (0-based): doubles each time, capped. */
export function backoffDelayMs(
  attempt: number,
  base = BACKOFF_BASE_MS,
  max = BACKOFF_MAX_MS,
): number {
  const safe = Math.max(0, Math.floor(attempt))
  return Math.min(max, base * 2 ** Math.min(safe, 30))
}

/** True for failures that mean the API is unreachable, not that a request was rejected. */
export function isConnectivityError(error: unknown): boolean {
  if (error instanceof ApiError) {
    return GATEWAY_STATUSES.has(error.status)
  }
  return error instanceof TypeError
}

/** Appends a failure and returns the timestamps still inside the window. */
export function recordFailure(
  failures: readonly number[],
  now: number,
  windowMs = OFFLINE_FAILURE_WINDOW_MS,
): number[] {
  return [...failures, now].filter((t) => now - t <= windowMs)
}

export function shouldGoOffline(
  failures: readonly number[],
  threshold = OFFLINE_FAILURE_THRESHOLD,
): boolean {
  return failures.length >= threshold
}

/** Accepts only same-origin absolute paths, so a crafted link cannot redirect off-site. */
export function safeReturnPath(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined
  }
  if (
    !value.startsWith('/') ||
    value.startsWith('//') ||
    value.includes('\\')
  ) {
    return undefined
  }
  if (value === '/login' || value.startsWith('/login?')) {
    return undefined
  }
  return value
}
