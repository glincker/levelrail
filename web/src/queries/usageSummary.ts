// GET /api/v1/usage/summary (internal/api/usage_summary.go): database
// CPU and memory readings, local volume sizes and stored backup bytes.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DatabaseUsage {
  name: string
  cpu_percent?: number
  memory_usage_bytes?: number
  memory_limit_bytes?: number
}

export interface VolumeUsageItem {
  name: string
  owner_kind: 'app' | 'database' | 'other'
  owner?: string
  size_bytes?: number
}

export interface BackupUsageItem {
  kind: 'app' | 'database'
  name: string
  count: number
  bytes: number
}

export interface UsageSummary {
  local_cpu_cores: number
  databases: DatabaseUsage[]
  volumes: {
    scope: string
    total_bytes: number
    unknown_count: number
    items: VolumeUsageItem[]
    not_configured?: boolean
  }
  backups: {
    total_bytes: number
    count: number
    items: BackupUsageItem[]
  }
}

export const usageSummaryKeys = {
  all: ['usage', 'summary'] as const,
}

export async function fetchUsageSummary(): Promise<UsageSummary> {
  const res = await fetch('/api/v1/usage/summary')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch usage summary failed: ${res.status}`),
    )
  }
  return (await res.json()) as UsageSummary
}

export function usageSummaryQueryOptions() {
  return queryOptions({
    queryKey: usageSummaryKeys.all,
    queryFn: fetchUsageSummary,
  })
}

const POLL_INTERVAL_MS = 60_000

// refetchIntervalInBackground stays false: a hidden tab does not poll.
export function useUsageSummary() {
  return useQuery({
    ...usageSummaryQueryOptions(),
    retry: false,
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  })
}
