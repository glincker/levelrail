import { useSyncExternalStore } from 'react'

const KEY = 'update-strip:dismissed'
const MAX_KEPT = 10
const listeners = new Set<() => void>()
let memory: string[] = []
let rev = 0

function read(): string[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(KEY) ?? '[]')
    return Array.isArray(parsed)
      ? parsed.filter((x): x is string => typeof x === 'string')
      : []
  } catch {
    return []
  }
}

export function isDismissed(version: string): boolean {
  return read().includes(version) || memory.includes(version)
}

export function dismissUpdate(version: string): void {
  if (!version || isDismissed(version)) return
  memory = [...memory, version].slice(-MAX_KEPT)
  try {
    localStorage.setItem(
      KEY,
      JSON.stringify([...read(), version].slice(-MAX_KEPT)),
    )
  } catch {
    // The in-memory copy still hides it for this tab.
  }
  rev += 1
  listeners.forEach((l) => l())
}

export function resetDismissedForTests(): void {
  memory = []
  rev += 1
  try {
    localStorage.removeItem(KEY)
  } catch {
    // ignore
  }
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function useUpdateDismissed(version: string): boolean {
  useSyncExternalStore(
    subscribe,
    () => rev,
    () => 0,
  )
  return version !== '' && isDismissed(version)
}
