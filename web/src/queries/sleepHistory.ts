// GET /api/v1/apps/{name}/sleep/history (internal/api/app_sleep_history.go):
// slept and woke events with aggregate stats.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { IdleLimits } from './appSleep'

export type SleepEventKind =
  'slept' | 'woke' | 'enabled' | 'disabled' | 'wake_failed'

export interface SleepEvent {
  id: number
  kind: SleepEventKind
  at: string
  reason?: string
  idle_minutes?: number
  trigger_path?: string
  trigger_source?: string
  detail?: string
  duration_seconds?: number
  wake_latency_ms?: number
  actor: string
}

export interface SleepStats {
  currently_asleep_since?: string
  asleep_seconds_7d: number
  asleep_seconds_30d: number
  wake_count_7d: number
  avg_wake_latency_ms?: number
  p95_wake_latency_ms?: number
  last_slept_at?: string
  last_woke_at?: string
  last_wake_reason?: string
  memory_gib_hours_saved_30d?: number
}

export interface SleepHistory {
  enabled: boolean
  sleeping: boolean
  events: SleepEvent[]
  stats: SleepStats
  idle_limits: IdleLimits
}

const HISTORY_REFETCH_MS = 30_000
const HISTORY_LIMIT = 100

export const sleepHistoryKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'sleep-history'] as const,
}

export function sleepHistoryQueryOptions(appName: string) {
  return queryOptions({
    queryKey: sleepHistoryKeys.detail(appName),
    queryFn: async (): Promise<SleepHistory> => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/sleep/history?since=30d&limit=${HISTORY_LIMIT}`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch sleep history failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as SleepHistory
    },
    refetchInterval: HISTORY_REFETCH_MS,
  })
}

export function useSleepHistory(appName: string) {
  return useQuery(sleepHistoryQueryOptions(appName))
}
