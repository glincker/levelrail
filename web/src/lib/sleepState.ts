import type { AppSleep, IdleLimits, WakeTiming } from '../queries/appSleep'

export type SleepUiState = 'ready' | 'blocked' | 'awake' | 'sleeping' | 'waking'

export const IDLE_PRESETS = [5, 15, 30, 60] as const

/** deriveSleepState maps an API response, plus a client-side wake in flight, to one UI state. */
export function deriveSleepState(
  sleep: AppSleep,
  wakePending = false,
): SleepUiState {
  if (sleep.sleeping) return wakePending ? 'waking' : 'sleeping'
  if (wakePending) return 'waking'
  if (sleep.enabled) return 'awake'
  return sleep.eligible ? 'ready' : 'blocked'
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

/** isValidIdleMinutes reports whether a custom idle time is within the server's range. */
export function isValidIdleMinutes(value: number, limits: IdleLimits): boolean {
  return (
    Number.isInteger(value) &&
    value >= limits.min_minutes &&
    value <= limits.max_minutes
  )
}

export interface WakeEstimate {
  typicalMs: number
  p95Ms: number | null
}

/** wakeEstimate returns the measured wake time, or null while nothing is measured. */
export function wakeEstimate(timing: WakeTiming): WakeEstimate | null {
  if (timing.samples === 0 || timing.ready_p50_ms === undefined) return null
  return { typicalMs: timing.ready_p50_ms, p95Ms: timing.ready_p95_ms ?? null }
}

/** formatWakeMs renders a wake time as ms below a second, else seconds with one decimal. */
export function formatWakeMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${(ms / 1000).toFixed(1)} s`
}
