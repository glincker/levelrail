import { useSyncExternalStore } from 'react'
import { summarizeAppStatus } from '../../lib/appStatus'
import type { ReconcileCondition } from '../../types/deploy'

const SKIP_RESTART_KEY = 'app-power:skip-restart-confirm'

/** Per-browser opt-out for the restart confirm; stop is never skippable. */
export function readSkipRestartConfirm(): boolean {
  try {
    return window.localStorage.getItem(SKIP_RESTART_KEY) === '1'
  } catch {
    return false
  }
}

export function writeSkipRestartConfirm(skip: boolean): void {
  try {
    if (skip) window.localStorage.setItem(SKIP_RESTART_KEY, '1')
    else window.localStorage.removeItem(SKIP_RESTART_KEY)
  } catch {
    // Storage can be blocked; the confirm just keeps showing.
  }
}

export type PowerKind = 'restart' | 'stop'

export interface PowerTiming {
  /** Before this, a stale pre-action status must not count as the result. */
  graceMs: number
  timeoutMs: number
  pollMs: number
  /** A terminal failure must show on this many consecutive polls. */
  failPolls: number
}

export const POWER_TIMING: PowerTiming = {
  graceMs: Number(import.meta.env.VITE_APP_POWER_GRACE_MS ?? 4000),
  timeoutMs: Number(import.meta.env.VITE_APP_POWER_TIMEOUT_MS ?? 120000),
  pollMs: Number(import.meta.env.VITE_APP_POWER_POLL_MS ?? 2000),
  failPolls: Number(import.meta.env.VITE_APP_POWER_FAIL_POLLS ?? 3),
}

// The reconciler keeps these while it works toward healthy: backoff between
// restarts, probe still running, one-shot still running, a superseded pass.
const IN_PROGRESS_REASONS = new Set([
  'CrashLoopBackOff',
  'ReadinessPending',
  'RunningNotReady',
  'RunningToCompletion',
  'Superseded',
  'NodeRecovering',
])

/** First condition that is a real failure rather than the app still coming up. */
export function terminalFailure(
  conditions: ReconcileCondition[],
): ReconcileCondition | undefined {
  return conditions.find(
    (c) => c.Status === 'False' && !IN_PROGRESS_REASONS.has(c.Reason),
  )
}

export interface PowerTransition {
  kind: PowerKind
  startedAt: number
  seenBusy: boolean
  /** The wait ran past the timeout; still tracked, buttons re-enabled. */
  slow: boolean
  /** Consecutive polls that showed a terminal failure. */
  failStreak: number
  /** Query update stamp last counted, so a clock tick never counts twice. */
  lastPollAt: number
}

export type PowerOutcome =
  | { state: 'pending'; busy: boolean }
  | { state: 'done' }
  | { state: 'failed'; reason: string }

/** Reads the live conditions to decide whether the action has finished. */
export function evaluatePower(
  transition: PowerTransition,
  conditions: ReconcileCondition[],
  now: number,
  timing: PowerTiming = POWER_TIMING,
): PowerOutcome {
  const label = summarizeAppStatus(conditions).label
  const settled =
    transition.seenBusy || now - transition.startedAt >= timing.graceMs
  if (transition.kind === 'stop') {
    return label === 'Stopped'
      ? { state: 'done' }
      : { state: 'pending', busy: false }
  }
  if (label === 'Healthy' && settled) return { state: 'done' }
  if ((label === 'Attention needed' || label === 'Node offline') && settled) {
    const bad = terminalFailure(conditions)
    if (bad) return { state: 'failed', reason: `${bad.Reason}: ${bad.Message}` }
  }
  return { state: 'pending', busy: label !== 'Healthy' }
}

const transitions = new Map<string, PowerTransition>()
const listeners = new Set<() => void>()

function emit() {
  listeners.forEach((l) => {
    l()
  })
}

export function beginPowerTransition(
  name: string,
  kind: PowerKind,
  now = Date.now(),
): void {
  transitions.set(name, {
    kind,
    startedAt: now,
    seenBusy: false,
    slow: false,
    failStreak: 0,
    lastPollAt: 0,
  })
  emit()
}

export function updatePowerTransition(
  name: string,
  patch: Partial<
    Pick<PowerTransition, 'seenBusy' | 'slow' | 'failStreak' | 'lastPollAt'>
  >,
): void {
  const cur = transitions.get(name)
  if (!cur) return
  transitions.set(name, { ...cur, ...patch })
  emit()
}

/** True only for the caller that actually removed it, so a toast fires once. */
export function endPowerTransition(name: string): boolean {
  const had = transitions.delete(name)
  if (had) emit()
  return had
}

export function getPowerTransition(name: string): PowerTransition | undefined {
  return transitions.get(name)
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function usePowerTransition(name: string): PowerTransition | undefined {
  return useSyncExternalStore(
    subscribe,
    () => transitions.get(name),
    () => undefined,
  )
}

export function resetPowerTransitions(): void {
  transitions.clear()
  emit()
}
