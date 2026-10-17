// GET /api/v1/sleep (internal/api/app_sleep_fleet.go): apps with sleep when
// idle on, how many are asleep now, and the memory limits they are not using.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { WakeTiming } from './appSleep'

export interface SleepFleetApp {
  name: string
  idle_minutes: number
  sleeping: boolean
  memory_limit_bytes: number
  wake_timing: WakeTiming
}

export interface SleepFleet {
  enabled: number
  sleeping: number
  reclaimed_bytes: number
  apps: SleepFleetApp[]
}

export const sleepFleetKey = ['sleep-fleet'] as const

export function sleepFleetQueryOptions() {
  return queryOptions({
    queryKey: sleepFleetKey,
    queryFn: async (): Promise<SleepFleet> => {
      const res = await fetch('/api/v1/sleep')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch sleep fleet failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as SleepFleet
    },
    refetchInterval: 30_000,
  })
}

export function useSleepFleet() {
  return useQuery(sleepFleetQueryOptions())
}
