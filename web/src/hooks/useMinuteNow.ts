import { useEffect, useState } from 'react'
import { setVisibleInterval } from '../lib/pageVisibility'

export function useMinuteNow(): number {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const stop = setVisibleInterval(() => setNow(Date.now()), 60_000)
    return () => {
      stop()
    }
  }, [])
  return now
}
