import { useCallback, useState } from 'react'

function read(key: string, fallback: boolean): boolean {
  try {
    const raw = window.localStorage.getItem(key)
    return raw === null ? fallback : raw === '1'
  } catch {
    return fallback
  }
}

// A per-browser boolean (e.g. a "More options" fold) that survives reloads.
// Storage can throw in locked-down contexts, so it degrades to plain state.
export function usePersistedFlag(
  key: string,
  fallback = false,
): [boolean, (next: boolean | ((prev: boolean) => boolean)) => void] {
  const [value, setValue] = useState(() => read(key, fallback))
  const set = useCallback(
    (next: boolean | ((prev: boolean) => boolean)) => {
      setValue((prev) => {
        const resolved = typeof next === 'function' ? next(prev) : next
        try {
          window.localStorage.setItem(key, resolved ? '1' : '0')
        } catch {
          // Not persisting is fine, the fold still works for this session.
        }
        return resolved
      })
    },
    [key],
  )
  return [value, set]
}
