import { useQuery } from '@tanstack/react-query'
import type { AppReleasesResponse } from '../types/appRelease'
import { appKeys } from './apps'

export const appReleasesKeys = {
  list: (appName: string) => [...appKeys.detail(appName), 'releases'] as const,
}

// 404/501 mean release tracking is off; treated as an empty list so the
// section simply stays hidden.
export async function fetchAppReleases(
  appName: string,
): Promise<AppReleasesResponse> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/releases`,
  )
  if (res.status === 404 || res.status === 501) return { releases: [] }
  if (!res.ok) throw new Error(`fetch app releases failed: ${res.status}`)
  return (await res.json()) as AppReleasesResponse
}

export function useAppReleases(appName: string) {
  return useQuery({
    queryKey: appReleasesKeys.list(appName),
    queryFn: () => fetchAppReleases(appName),
    retry: false,
  })
}
