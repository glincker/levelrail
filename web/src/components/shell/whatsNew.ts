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
  toasted = undefined
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

export const TOASTED_VERSION_KEY = 'shell.whatsnew.toastedVersion'

let toasted: string | undefined

function loadToasted(): string {
  toasted ??= readJson(TOASTED_VERSION_KEY, '', isString)
  return toasted
}

// One quiet "Updated to X" toast per version, never on a first-ever load
// (lastSeen empty) and never again once shown.
export function shouldToastUpgrade(latest: string, lastSeen: string): boolean {
  return (
    latest !== '' &&
    lastSeen !== '' &&
    lastSeen !== latest &&
    loadToasted() !== latest
  )
}

export function markUpgradeToasted(version: string): void {
  toasted = version
  writeJson(TOASTED_VERSION_KEY, version)
}

const openRequests = new Set<() => void>()

// Lets the toast CTA open whichever panel instance is currently visible.
export function onWhatsNewOpenRequest(cb: () => void): () => void {
  openRequests.add(cb)
  return () => {
    openRequests.delete(cb)
  }
}

export function requestOpenWhatsNew(): void {
  openRequests.forEach((cb) => {
    cb()
  })
}
