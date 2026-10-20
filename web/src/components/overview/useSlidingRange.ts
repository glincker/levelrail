import { useEffect, useState } from 'react'

/** A [now - minutes, now] range that only moves every refreshMs, so query keys stay stable. */
export function useSlidingRange(minutes: number, refreshMs: number) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), refreshMs)
    return () => window.clearInterval(id)
  }, [refreshMs])
  return { from: new Date(now - minutes * 60_000), to: new Date(now) }
}
