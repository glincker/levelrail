import { useCallback, useMemo } from 'react'
import { NAV_PREFS_STORAGE_KEY, NAV_RECENT_STORAGE_KEY } from '@/lib/navConfig'
import { parseNavPrefs, type NavPrefs } from '@/lib/navPrefs'
import { readNavRaw, useNavPrefsRaw, writeNavRaw } from './useNavPrefs'

// Pins are unpinned from their own control, so Reset leaves them alone.
export function resetNavPersonalization() {
  writeNavRaw(NAV_PREFS_STORAGE_KEY, null)
  writeNavRaw(NAV_RECENT_STORAGE_KEY, null)
}

// Dialog-only: parsed prefs plus a writer, kept out of the eager sidebar code.
export function useNavPrefs() {
  const raw = useNavPrefsRaw()
  const prefs = useMemo(() => parseNavPrefs(raw), [raw])
  const update = useCallback((fn: (p: NavPrefs) => NavPrefs) => {
    const next = fn(parseNavPrefs(readNavRaw(NAV_PREFS_STORAGE_KEY)))
    writeNavRaw(NAV_PREFS_STORAGE_KEY, JSON.stringify(next))
  }, [])
  return { prefs, update }
}
