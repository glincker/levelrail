// Parked domain auto cleanup preview: GET /api/v1/domains/parked/retention.
import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { RetentionCandidate } from '../lib/parkedRetention'
import { pollMs } from '@/lib/pollTier'

export interface ParkedRetention {
  retention_days: number
  would_delete: RetentionCandidate[]
  total_eligible: number
  batch_limit: number
  // Omitted while retention is off.
  next_run?: string
}

export const RETENTION_SETTING_KEY = 'domains.parked_retention_days'
export const RETENTION_REFETCH_MS = pollMs('idle')

export const parkedRetentionKeys = {
  all: ['domains', 'parked', 'retention'] as const,
}

// Resolves to null on 403 so the strip can hide for read-only roles.
export function parkedRetentionQueryOptions() {
  return queryOptions({
    queryKey: parkedRetentionKeys.all,
    refetchInterval: RETENTION_REFETCH_MS,
    retry: false,
    queryFn: async (): Promise<ParkedRetention | null> => {
      const res = await fetch('/api/v1/domains/parked/retention')
      if (res.status === 403) return null
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `parked retention failed: ${res.status}`),
        )
      }
      return (await res.json()) as ParkedRetention
    },
  })
}

export function useParkedRetention() {
  return useQuery(parkedRetentionQueryOptions())
}
