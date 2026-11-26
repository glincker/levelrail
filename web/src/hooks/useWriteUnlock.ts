import { useCallback, useEffect, useState, useSyncExternalStore } from 'react'

// Minutes a write unlock lasts; 0 means it stays until locked again.
export const DEFAULT_UNLOCK_MINUTES = Number(
  import.meta.env.VITE_DB_WRITE_UNLOCK_MINUTES ?? 15,
)
// One second keeps the countdown and the auto relock honest.
const TICK_MS = 1000
const listeners = new Set<() => void>()

const keyFor = (db: string) => `lr.dbWrite.${db}`

function read(db: string): string | null {
  try {
    return window.sessionStorage.getItem(keyFor(db))
  } catch {
    return null
  }
}

function write(db: string, value: string | null) {
  try {
    if (value === null) window.sessionStorage.removeItem(keyFor(db))
    else window.sessionStorage.setItem(keyFor(db), value)
  } catch {
    // Storage blocked: the unlock then only lasts until the next reload.
  }
  listeners.forEach((l) => {
    l()
  })
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}

export interface WriteUnlock {
  unlocked: boolean
  /** Epoch ms when the window ends, null when it has no end. */
  until: number | null
  /** Minutes override the env default; 0 keeps it unlocked until locked. */
  unlock: (minutes?: number) => void
  lock: () => void
}

/** One write unlock per database for this browser tab, shared by Tables, Query and the status strip. The server still enforces writes. */
export function useWriteUnlock(db: string): WriteUnlock {
  const raw = useSyncExternalStore(subscribe, () => read(db))
  const [now, setNow] = useState(() => Date.now())
  const until = raw && raw !== '0' ? Number(raw) : null
  const expired = until !== null && until <= now
  const unlocked = raw !== null && !expired

  useEffect(() => {
    if (!unlocked) return
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, TICK_MS)
    return () => {
      window.clearInterval(id)
    }
  }, [unlocked])

  useEffect(() => {
    if (expired) write(db, null)
  }, [expired, db])

  const unlock = useCallback(
    (minutes: number = DEFAULT_UNLOCK_MINUTES) => {
      const ms = minutes > 0 ? Date.now() + minutes * 60_000 : 0
      setNow(Date.now())
      write(db, String(ms))
    },
    [db],
  )
  const lock = useCallback(() => {
    write(db, null)
  }, [db])

  return { unlocked, until: unlocked ? until : null, unlock, lock }
}
