import { useCallback, useSyncExternalStore } from 'react'
import {
  EMPTY_NAV_PREFS,
  NAV_PREFS_STORAGE_KEY,
  NAV_RECENT_STORAGE_KEY,
  parseNavPrefs,
  parseRecent,
  pushRecent,
  type NavPrefs,
  type RecentEntry,
} from '@/lib/navPrefs'

// Account preference sync only accepts appearance keys, so sidebar layout
// stays in this browser's localStorage.
const listeners = new Set<() => void>()
const NO_RECENT: RecentEntry[] = []
let prefsRaw: string | null | undefined
let prefsCache: NavPrefs = EMPTY_NAV_PREFS
let recentRaw: string | null | undefined
let recentCache: RecentEntry[] = NO_RECENT

function readRaw(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function writeRaw(key: string, value: string | null) {
  try {
    if (value === null) window.localStorage.removeItem(key)
    else window.localStorage.setItem(key, value)
  } catch {
    // Storage unavailable: the change lasts for this session only.
  }
}

function emit() {
  listeners.forEach((l) => l())
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  const onStorage = (e: StorageEvent) => {
    if (e.key === null || e.key.startsWith('shell.nav.')) cb()
  }
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(cb)
    window.removeEventListener('storage', onStorage)
  }
}

function getPrefs(): NavPrefs {
  const raw = readRaw(NAV_PREFS_STORAGE_KEY)
  if (raw !== prefsRaw) {
    prefsRaw = raw
    prefsCache = parseNavPrefs(raw)
  }
  return prefsCache
}

function getRecent(): RecentEntry[] {
  const raw = readRaw(NAV_RECENT_STORAGE_KEY)
  if (raw !== recentRaw) {
    recentRaw = raw
    recentCache = parseRecent(raw)
  }
  return recentCache
}

export function useNavPrefs() {
  const prefs = useSyncExternalStore(subscribe, getPrefs, getPrefs)
  const setPrefs = useCallback((next: NavPrefs) => {
    writeRaw(NAV_PREFS_STORAGE_KEY, JSON.stringify(next))
    emit()
  }, [])
  const update = useCallback(
    (fn: (p: NavPrefs) => NavPrefs) => setPrefs(fn(getPrefs())),
    [setPrefs],
  )
  return { prefs, update }
}

export function useNavRecent() {
  const recent = useSyncExternalStore(subscribe, getRecent, getRecent)
  const record = useCallback((entry: RecentEntry) => {
    const next = pushRecent(getRecent(), entry)
    if (JSON.stringify(next) === JSON.stringify(getRecent())) return
    writeRaw(NAV_RECENT_STORAGE_KEY, JSON.stringify(next))
    emit()
  }, [])
  return { recent, record }
}

// Pins are unpinned from their own control, so Reset leaves them alone.
export function resetNavPersonalization() {
  writeRaw(NAV_PREFS_STORAGE_KEY, null)
  writeRaw(NAV_RECENT_STORAGE_KEY, null)
  emit()
}
