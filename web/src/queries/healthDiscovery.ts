// Fetcher and mutation hook for POST /api/v1/apps/{name}/health/discover
// (internal/api/apps_health_discover.go): actively probes a fixed set of
// well-known paths against an app's running container and reports every
// path's real outcome. Not cached (useMutation, not useQuery): each click
// is a fresh, real probe, never a stale result replayed from the cache.

import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface HealthDiscoveryAttempt {
  path: string
  success: boolean
  error?: string
  latency_ms: number
}

export interface HealthDiscoveryResult {
  name: string
  attempts: HealthDiscoveryAttempt[]
  // Set only when exactly one candidate path returned a clear 2xx.
  found?: string
}

export async function discoverAppHealth(
  name: string,
): Promise<HealthDiscoveryResult> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/health/discover`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `discover health failed: ${res.status}`),
    )
  }
  return (await res.json()) as HealthDiscoveryResult
}

export function useDiscoverAppHealth() {
  return useMutation({ mutationFn: discoverAppHealth })
}
