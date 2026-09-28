import * as React from 'react'
import { readJson, writeJson } from './persist'
import {
  isSnoozed,
  isSnoozeMap,
  pruneExpired,
  snoozeIssue,
  unsnoozeIssue,
  type SnoozeDuration,
  type SnoozeMap,
} from './snooze'

export const SNOOZE_STORAGE_KEY = 'shell.issues.snoozed'

const EMPTY: SnoozeMap = {}
let current: SnoozeMap | undefined
const listeners = new Set<() => void>()

function load(): SnoozeMap {
  current ??= pruneExpired(
    readJson(SNOOZE_STORAGE_KEY, EMPTY, isSnoozeMap),
    Date.now(),
  )
  return current
}

function commit(next: SnoozeMap): void {
  current = next
  writeJson(SNOOZE_STORAGE_KEY, next)
  listeners.forEach((l) => {
    l()
  })
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

export function snoozeNow(id: string, d: SnoozeDuration): void {
  commit(snoozeIssue(load(), id, d, Date.now()))
}

export function unsnoozeNow(id: string): void {
  commit(unsnoozeIssue(load(), id))
}

export function resetSnoozeStoreForTests(): void {
  current = undefined
}

const TICK_MS = 60_000

// Re-renders every minute so an expired snooze reappears without a reload.
export function useSnoozed(): {
  map: SnoozeMap
  now: number
  isSnoozed: (id: string) => boolean
} {
  const map = React.useSyncExternalStore(subscribe, load, load)
  const [now, setNow] = React.useState(() => Date.now())
  React.useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), TICK_MS)
    return () => window.clearInterval(t)
  }, [])
  return { map, now, isSnoozed: (id) => isSnoozed(map, id, now) }
}
