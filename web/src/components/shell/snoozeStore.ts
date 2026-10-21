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
import { setVisibleInterval } from '../../lib/pageVisibility'
import {
  isSilenceActive,
  useSilences,
  type IssueSilence,
} from '../../queries/silences'

export const SNOOZE_STORAGE_KEY = 'shell.issues.snoozed'

const EMPTY: SnoozeMap = {}
const NO_SILENCES: IssueSilence[] = []
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
// Server silences (shared by every operator) and the legacy per-browser
// snoozes both hide an issue; the header count and Status page read this.
export function useSnoozed(): {
  map: SnoozeMap
  now: number
  silences: IssueSilence[]
  isSnoozed: (id: string) => boolean
} {
  const { data } = useSilences()
  const silences = data ?? NO_SILENCES
  const map = React.useSyncExternalStore(subscribe, load, load)
  const [now, setNow] = React.useState(() => Date.now())
  React.useEffect(() => {
    const stop = setVisibleInterval(() => setNow(Date.now()), TICK_MS)
    return () => stop()
  }, [])
  return {
    map,
    now,
    silences,
    isSnoozed: (id) =>
      isSnoozed(map, id, now) ||
      silences.some((s) => s.issue_key === id && isSilenceActive(s, now)),
  }
}
