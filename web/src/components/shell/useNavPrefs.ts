import { useEffect, useSyncExternalStore } from 'react'
import { NAV_PREFS_STORAGE_KEY, NAV_RECENT_STORAGE_KEY } from '@/lib/navConfig'

// Eager and tiny: it only moves raw strings. Parsing and ordering live in
// lib/navPrefs, which loads on demand. Account sync accepts only appearance
// keys, so sidebar layout stays in this browser's localStorage.
const listeners = new Set<() => void>()

export function readNavRaw(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

export function writeNavRaw(key: string, value: string | null) {
  try {
    if (value === null) window.localStorage.removeItem(key)
    else window.localStorage.setItem(key, value)
  } catch {
    // Storage unavailable: the change lasts for this session only.
  }
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

const prefsRaw = () => readNavRaw(NAV_PREFS_STORAGE_KEY)
const recentRaw = () => readNavRaw(NAV_RECENT_STORAGE_KEY)

export function useNavPrefsRaw() {
  return useSyncExternalStore(subscribe, prefsRaw, prefsRaw)
}

export function useNavRecentRaw() {
  return useSyncExternalStore(subscribe, recentRaw, recentRaw)
}

// Records an app or database detail visit; the matching code loads only then.
export function useRecordRecent(pathname: string) {
  useEffect(() => {
    if (!/^\/(apps|databases)\/[^/]/.test(pathname)) return
    void import('@/lib/navPrefs').then((m) => {
      const next = m.recordRecent(pathname, recentRaw())
      if (next !== null) writeNavRaw(NAV_RECENT_STORAGE_KEY, next)
    })
  }, [pathname])
}
