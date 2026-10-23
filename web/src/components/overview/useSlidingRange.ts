import { useEffect, useState } from 'react'
import { setVisibleInterval } from '../../lib/pageVisibility'

/** A [now - minutes, now] range that only moves every refreshMs, so query keys stay stable. */
export function useSlidingRange(minutes: number, refreshMs: number) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const stop = setVisibleInterval(() => setNow(Date.now()), refreshMs)
    return () => stop()
  }, [refreshMs])
  return { from: new Date(now - minutes * 60_000), to: new Date(now) }
}
