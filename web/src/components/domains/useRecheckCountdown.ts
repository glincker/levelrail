import { useEffect, useRef, useState } from 'react'

// Interval comes from env like the other tunables; 45s keeps load low.
export const RECHECK_SECONDS = Number(
  import.meta.env.VITE_DOMAIN_RECHECK_SECONDS ?? 45,
)

// Counts down to the next check, pauses while the tab is hidden and restarts
// whenever `resetKey` changes (a fresh result arrived from anywhere).
export function useRecheckCountdown({
  active,
  resetKey,
  onFire,
  seconds = RECHECK_SECONDS,
}: {
  active: boolean
  resetKey?: string
  onFire: () => void
  seconds?: number
}): number {
  const [tick, setTick] = useState({ key: resetKey, left: seconds })
  const fire = useRef(onFire)
  useEffect(() => {
    fire.current = onFire
  })
  const left = tick.key === resetKey ? tick.left : seconds
  const leftRef = useRef(left)
  useEffect(() => {
    leftRef.current = left
  })
  useEffect(() => {
    if (!active) return undefined
    const id = setInterval(() => {
      if (document.hidden) return
      const next = leftRef.current - 1
      if (next <= 0) {
        fire.current()
        setTick({ key: resetKey, left: seconds })
      } else {
        setTick({ key: resetKey, left: next })
      }
    }, 1000)
    return () => {
      clearInterval(id)
    }
  }, [active, resetKey, seconds])
  return left
}
