import * as React from 'react'
import { readJson, writeJson } from './persist'
import type { ChangelogEntry } from '../../queries/changelog'

export const LAST_SEEN_VERSION_KEY = 'shell.whatsnew.lastSeenVersion'

const isString = (v: unknown): v is string => typeof v === 'string'

let current: string | undefined
const listeners = new Set<() => void>()

function load(): string {
  current ??= readJson(LAST_SEEN_VERSION_KEY, '', isString)
  return current
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

// "" means never opened: every entry counts as unread until a real
// version is recorded.
export function setLastSeenVersion(version: string): void {
  current = version
  writeJson(LAST_SEEN_VERSION_KEY, version)
  listeners.forEach((l) => {
    l()
  })
}

export function resetWhatsNewStoreForTests(): void {
  current = undefined
}

// useLastSeenVersion is the version the operator last opened the
// "what's new" panel at, "" meaning never.
export function useLastSeenVersion(): string {
  return React.useSyncExternalStore(subscribe, load, load)
}

// unreadCount counts how many of entries (newest-first) are newer than
// lastSeenVersion: every entry when lastSeenVersion is "" (never
// opened), zero once the latest entry's version matches it, and the
// whole list when lastSeenVersion matches nothing the server currently
// returns (an older release than it still keeps, or a stale value from
// a prior install).
export function unreadCount(
  entries: ChangelogEntry[],
  lastSeenVersion: string,
): number {
  if (lastSeenVersion === '') return entries.length
  const idx = entries.findIndex((e) => e.version === lastSeenVersion)
  return idx === -1 ? entries.length : idx
}
