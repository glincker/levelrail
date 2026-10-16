import { useSyncExternalStore } from 'react'

export interface UpgradeTrack {
  id: string
  target: string
  step: string
  startedAt: number
}

const STORAGE_KEY = 'upgrade-tracker'
let current: UpgradeTrack | null = readStored()
const listeners = new Set<() => void>()

function readStored(): UpgradeTrack | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    return raw ? (JSON.parse(raw) as UpgradeTrack) : null
  } catch {
    return null
  }
}

function set(next: UpgradeTrack | null) {
  if (JSON.stringify(next) === JSON.stringify(current)) return
  current = next
  try {
    if (next) sessionStorage.setItem(STORAGE_KEY, JSON.stringify(next))
    else sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // Storage blocked: in-memory state still drives this tab.
  }
  listeners.forEach((l) => l())
}

export const rememberUpgrade = (t: UpgradeTrack) => set(t)
export const clearUpgrade = () => set(null)

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function useUpgradeTrack(): UpgradeTrack | null {
  return useSyncExternalStore(
    subscribe,
    () => current,
    () => null,
  )
}
