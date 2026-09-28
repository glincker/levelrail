// Query-key factory and fetcher for GET /api/v1/updates
// (internal/api/updates.go's handleGetUpdates), the Settings > Updates
// page's own data source: running version vs. GitHub's latest published
// release.

import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface UpdateStatus {
  current_version: string
  latest_version: string | null
  update_available: boolean
  release_url: string | null
  published_at: string | null
}

export const updatesKeys = {
  all: ['updates'] as const,
  status: () => [...updatesKeys.all, 'status'] as const,
}

export async function fetchUpdateStatus(): Promise<UpdateStatus> {
  const res = await fetch('/api/v1/updates')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch updates failed: ${res.status}`),
    )
  }
  return (await res.json()) as UpdateStatus
}

export function updatesQueryOptions() {
  return queryOptions({
    queryKey: updatesKeys.status(),
    queryFn: fetchUpdateStatus,
  })
}

export type UpgradeCheckStatus = 'ok' | 'warn' | 'fail' | 'unknown'

export interface UpgradeCheck {
  code: string
  name: string
  status: UpgradeCheckStatus
  message: string
}

export interface UpdatePreflight {
  current_version: string
  latest_version: string | null
  update_available: boolean
  release_url: string | null
  release_notes: string
  checks: UpgradeCheck[]
  blocked: boolean
  upgrade_command: string
  rollback_command: string
}

export function preflightQueryOptions() {
  return queryOptions({
    queryKey: [...updatesKeys.all, 'preflight'] as const,
    queryFn: async (): Promise<UpdatePreflight> => {
      const res = await fetch('/api/v1/updates/preflight')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `fetch preflight failed: ${res.status}`),
        )
      }
      return (await res.json()) as UpdatePreflight
    },
    staleTime: 60_000,
    retry: false,
  })
}
