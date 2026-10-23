import { useSyncExternalStore } from 'react'

export type SeenKind = 'toast' | 'panel' | 'card'

const KEY_PREFIX = 'upgrade-seen:'
const MAX_KEPT = 20
const listeners = new Set<() => void>()
let version = 0

function read(kind: SeenKind): string[] {
  try {
    const raw = localStorage.getItem(KEY_PREFIX + kind)
    const parsed: unknown = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed)
      ? parsed.filter((x): x is string => typeof x === 'string')
      : []
  } catch {
    return []
  }
}

// Falls back to memory when storage is blocked, so a tab still stays quiet.
const memory: Record<SeenKind, string[]> = { toast: [], panel: [], card: [] }

export function isSeen(kind: SeenKind, id: string): boolean {
  return read(kind).includes(id) || memory[kind].includes(id)
}

export function markSeen(kind: SeenKind, id: string): void {
  if (!id || isSeen(kind, id)) return
  memory[kind] = [...memory[kind], id].slice(-MAX_KEPT)
  try {
    localStorage.setItem(
      KEY_PREFIX + kind,
      JSON.stringify([...read(kind), id].slice(-MAX_KEPT)),
    )
  } catch {
    // Memory copy above still applies for this tab.
  }
  version += 1
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function useSeen(kind: SeenKind, id: string): boolean {
  useSyncExternalStore(
    subscribe,
    () => version,
    () => 0,
  )
  return id !== '' && isSeen(kind, id)
}
