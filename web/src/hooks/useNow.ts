import { useEffect, useState } from 'react'

// Re-renders on an interval so countdowns tick; paused while the tab is
// hidden so a background tab does no work.
export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const tick = () => {
      if (document.visibilityState === 'visible') {
        setNow(Date.now())
      }
    }
    const id = window.setInterval(tick, intervalMs)
    document.addEventListener('visibilitychange', tick)
    return () => {
      window.clearInterval(id)
      document.removeEventListener('visibilitychange', tick)
    }
  }, [intervalMs])
  return now
}
