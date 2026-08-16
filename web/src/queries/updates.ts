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
