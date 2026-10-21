import { useEffect, useState } from 'react'
import { setVisibleInterval } from '../lib/pageVisibility'

// Ticks `now` once a second while active, otherwise stays fixed at
// whatever it last was. Shared by anything rendering a live-counting
// duration (DeployStageTimeline's per-stage elapsed display,
// DeploySection's own elapsed display, the deploy detail page's overall
// duration), so they all tick on the same clock instead of each rolling
// its own interval.
export function useNowTick(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!active) return
    const stop = setVisibleInterval(() => setNow(Date.now()), 1000)
    return () => stop()
  }, [active])

  return now
}
