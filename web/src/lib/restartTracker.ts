import { useSyncExternalStore } from 'react'

export type RestartPhase = 'restarting' | 'back' | 'timeout'

export interface RestartState {
  phase: RestartPhase
  oldBootId: string
  startedAt: number
  sawDown: boolean
}

export type RestartProbe = { ok: false } | { ok: true; bootId: string }

export const RESTART_TIMEOUT_MS = 120_000
const STORAGE_KEY = 'restart-tracker'

// A restart is only "done" when a process with a different boot id
// answers: the old one keeps serving while it drains, so a bare health
// check would report success too early.
export function nextRestartState(
  state: RestartState,
  probe: RestartProbe,
  now: number,
): RestartState {
  if (state.phase !== 'restarting') return state
  if (!probe.ok) {
    return state.sawDown ? state : { ...state, sawDown: true }
  }
  if (probe.bootId !== state.oldBootId) return { ...state, phase: 'back' }
  if (now - state.startedAt > RESTART_TIMEOUT_MS) {
    return { ...state, phase: 'timeout' }
  }
  return state
}

export function backoffDelay(attempt: number): number {
  return Math.min(500 * 1.4 ** attempt, 2_000)
}

let current: RestartState | null = readStored()
const listeners = new Set<() => void>()

function readStored(): RestartState | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    return raw ? (JSON.parse(raw) as RestartState) : null
  } catch {
    return null
  }
}

function set(next: RestartState | null) {
  current = next
  try {
    if (next) sessionStorage.setItem(STORAGE_KEY, JSON.stringify(next))
    else sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // Storage blocked: the in-memory state still drives this tab.
  }
  listeners.forEach((l) => l())
}

export function beginRestart(oldBootId: string, now = Date.now()) {
  set({ phase: 'restarting', oldBootId, startedAt: now, sawDown: false })
}

export function applyProbe(probe: RestartProbe, now = Date.now()) {
  if (!current) return
  const next = nextRestartState(current, probe, now)
  if (next !== current) set(next)
}

export function clearRestart() {
  set(null)
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function useRestartState(): RestartState | null {
  return useSyncExternalStore(
    subscribe,
    () => current,
    () => null,
  )
}
