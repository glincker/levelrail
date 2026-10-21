import { useEffect, useState } from 'react'
import { setVisibleInterval } from './pageVisibility'

export function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const m = Math.floor(total / 60)
  const s = total % 60
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

export function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    const stop = setVisibleInterval(() => setNow(Date.now()), 1000)
    return () => stop()
  }, [active])
  return now
}
