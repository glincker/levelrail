import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { appListQueryOptions } from '../../queries/apps'
import {
  APP_POLL_BUDGET_MS,
  APP_POLL_INTERVAL_MS,
  APP_SLOW_AFTER_MS,
  SAMPLE_APP,
  firstAppPhase,
  pickTrackedApp,
  pollInterval,
} from '../../lib/firstApp'
import { setVisibleInterval } from '../../lib/pageVisibility'

const CLOCK_TICK_MS = 5_000

function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const stop = setVisibleInterval(() => setNow(Date.now()), CLOCK_TICK_MS)
    return () => stop()
  }, [])
  return now
}

/** useFirstAppPolling tracks the first app until it is healthy or the poll budget runs out. */
export function useFirstAppPolling(trackingSince: number, enabled = true) {
  const apps = useQuery({
    ...appListQueryOptions(),
    enabled,
    refetchInterval: (q) => {
      const app = pickTrackedApp(q.state.data ?? [], SAMPLE_APP.name)
      const live =
        firstAppPhase(app, trackingSince, Date.now(), APP_SLOW_AFTER_MS) ===
        'live'
      return pollInterval(
        live,
        trackingSince,
        Date.now(),
        APP_POLL_INTERVAL_MS,
        APP_POLL_BUDGET_MS,
      )
    },
  })
  const now = useNow()
  const app = pickTrackedApp(apps.data ?? [], SAMPLE_APP.name)
  const phase = firstAppPhase(app, trackingSince, now, APP_SLOW_AFTER_MS)
  return { app, phase, isLoading: apps.isLoading }
}
