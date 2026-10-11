import { useCallback, useEffect, useRef, useState } from 'react'

// Revealed values live only in component state: they auto-hide after the
// server's hide_after_seconds and vanish when the page unmounts.
export function useRevealedValues() {
  const [values, setValues] = useState<Record<string, string>>({})
  const [expiresAt, setExpiresAt] = useState<Record<string, number>>({})
  const timers = useRef(new Map<string, number>())

  const hide = useCallback((key: string) => {
    const timer = timers.current.get(key)
    if (timer !== undefined) {
      window.clearTimeout(timer)
      timers.current.delete(key)
    }
    setValues((prev) => {
      const next = { ...prev }
      delete next[key]
      return next
    })
    setExpiresAt((prev) => {
      const next = { ...prev }
      delete next[key]
      return next
    })
  }, [])

  const reveal = useCallback(
    (key: string, value: string, hideAfterSeconds: number) => {
      const existing = timers.current.get(key)
      if (existing !== undefined) window.clearTimeout(existing)
      const ms = Math.max(1, hideAfterSeconds) * 1000
      timers.current.set(
        key,
        window.setTimeout(() => {
          hide(key)
        }, ms),
      )
      setValues((prev) => ({ ...prev, [key]: value }))
      setExpiresAt((prev) => ({ ...prev, [key]: Date.now() + ms }))
    },
    [hide],
  )

  const hideAll = useCallback(() => {
    for (const timer of timers.current.values()) window.clearTimeout(timer)
    timers.current.clear()
    setValues({})
    setExpiresAt({})
  }, [])

  useEffect(() => {
    const pending = timers.current
    const onHidden = () => {
      if (document.visibilityState === 'hidden') hideAll()
    }
    document.addEventListener('visibilitychange', onHidden)
    return () => {
      document.removeEventListener('visibilitychange', onHidden)
      for (const timer of pending.values()) window.clearTimeout(timer)
      pending.clear()
    }
  }, [hideAll])

  return { values, expiresAt, reveal, hide, hideAll }
}
