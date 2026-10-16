import type { AppSleep, SleepIssue } from '../queries/appSleep'

export type SleepUiState =
  'off' | 'ready' | 'blocked' | 'awake' | 'sleeping' | 'waking'

export const MIN_IDLE_MINUTES = 5
export const MAX_IDLE_MINUTES = 10080
export const IDLE_PRESETS = [5, 15, 30, 60] as const

/** sleepBlockers returns the API blockers, or the deprecated blocked_reason as one blocker from an older API. */
export function sleepBlockers(sleep: AppSleep): SleepIssue[] {
  if (sleep.blockers) return sleep.blockers
  if (sleep.blocked_reason) {
    return [{ code: sleep.blocked_reason, title: '', detail: '' }]
  }
  return []
}

/** deriveSleepState maps an API response, plus a client-side wake in flight, to one UI state. */
export function deriveSleepState(
  sleep: AppSleep,
  wakePending = false,
): SleepUiState {
  if (sleep.sleeping) return wakePending ? 'waking' : 'sleeping'
  if (wakePending) return 'waking'
  if (sleep.enabled) return 'awake'
  if (sleepBlockers(sleep).length > 0 || sleep.eligible === false) {
    return 'blocked'
  }
  return sleep.eligible === true ? 'ready' : 'off'
}

export interface Countdown {
  unit: 'seconds' | 'minutes'
  value: number
}

/** sleepCountdown returns the time left until will_sleep_at, or null when none is scheduled. */
export function sleepCountdown(
  willSleepAt: string | undefined,
  nowMs: number,
): Countdown | null {
  if (!willSleepAt) return null
  const at = Date.parse(willSleepAt)
  if (!Number.isFinite(at)) return null
  const seconds = Math.max(0, Math.ceil((at - nowMs) / 1000))
  if (seconds < 60) return { unit: 'seconds', value: seconds }
  return { unit: 'minutes', value: Math.ceil(seconds / 60) }
}

/** isValidIdleMinutes reports whether a custom idle time is within the API range. */
export function isValidIdleMinutes(value: number): boolean {
  return (
    Number.isInteger(value) &&
    value >= MIN_IDLE_MINUTES &&
    value <= MAX_IDLE_MINUTES
  )
}
