// Query-key factory and fetcher for GET /api/v1/apps/{name}/network
// (internal/api/network.go's handleGetAppNetwork): the live traffic path
// for the Network tab, container port plus whatever host port Docker
// currently has bound, so an operator can see how a request actually
// reaches this app right now, not just what was declared.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api/network.go's networkResource wire shape exactly.
export interface AppNetwork {
  container_port: number
  host_port?: number
  running: boolean
}

export const appNetworkKeys = {
  detail: (appName: string) => ['apps', appName, 'network'] as const,
}

export async function fetchAppNetwork(appName: string): Promise<AppNetwork> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/network`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app network failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppNetwork
}

// 10s, live-status pace: this is observed state that changes on every
// container recreate, not a rare event like a cert renewal, so it's
// worth a short fixed poll the same way activity.ts's own
// ACTIVITY_POLL_INTERVAL_MS keeps a header widget current.
const APP_NETWORK_POLL_INTERVAL_MS = 10_000

export function appNetworkQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appNetworkKeys.detail(appName),
    queryFn: () => fetchAppNetwork(appName),
  })
}

export function useAppNetwork(appName: string) {
  return useQuery({
    ...appNetworkQueryOptions(appName),
    refetchInterval: APP_NETWORK_POLL_INTERVAL_MS,
  })
}
