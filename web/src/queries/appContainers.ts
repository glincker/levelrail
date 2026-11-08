// GET /api/v1/apps/{name}/containers: per-replica details, polled for live stats.
import { queryOptions, useQuery } from '@tanstack/react-query'
import type { AppContainer } from '../types/appContainers'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'

const POLL_MS = Number(import.meta.env.VITE_APP_CONTAINERS_POLL_MS) || 5000

export const appContainersKeys = {
  list: (name: string) => [...appKeys.detail(name), 'containers'] as const,
}

export async function fetchAppContainers(
  name: string,
): Promise<AppContainer[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/containers`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app containers failed: ${res.status}`),
    )
  }
  return ((await res.json()) as AppContainer[] | null) ?? []
}

export function appContainersQueryOptions(name: string) {
  return queryOptions({
    queryKey: appContainersKeys.list(name),
    queryFn: () => fetchAppContainers(name),
    // Background polling stops while the browser tab is hidden.
    refetchInterval: POLL_MS,
    refetchIntervalInBackground: false,
    retry: false,
  })
}

export function useAppContainers(name: string) {
  return useQuery(appContainersQueryOptions(name))
}
