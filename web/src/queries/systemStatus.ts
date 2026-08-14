// Query-key factory and fetcher for GET /api/v1/system/status
// (internal/api/status.go's handleSystemStatus): configured/not-
// configured signals for secrets/telemetry/alerts plus data-dir disk
// usage, the real content routes/settings/general.tsx's own comment
// said didn't exist yet. Requires a session (unlike brand.ts/devMode.ts,
// this isn't needed before login), so this is an ordinary suspense
// query primed by the settings/general route's own loader, not a
// public/optional one.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const systemStatusKeys = {
  all: ['system-status'] as const,
}

export interface SystemStatus {
  secrets_configured: boolean
  telemetry_configured: boolean
  alerts_configured: boolean
  data_dir_total_bytes?: number
  data_dir_free_bytes?: number
  docker_connected: boolean
}

export async function fetchSystemStatus(): Promise<SystemStatus> {
  const res = await fetch('/api/v1/system/status')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch system status failed: ${res.status}`),
    )
  }
  return (await res.json()) as SystemStatus
}

// staleTime matches devModeQueryOptions's own reasoning for the two
// "configured or not" booleans (fixed for the process lifetime, set by
// env vars at startup), less true of the two disk-usage fields (they
// genuinely change as the data directory fills up), but a settings page
// re-visited occasionally doesn't need live disk tracking, a fresh read
// on each navigation to this route is enough, so a moderate staleTime
// rather than Infinity or 0.
export function systemStatusQueryOptions() {
  return queryOptions({
    queryKey: systemStatusKeys.all,
    queryFn: fetchSystemStatus,
    staleTime: 60_000,
  })
}

export function useSystemStatus() {
  return useSuspenseQuery(systemStatusQueryOptions())
}
