import { queryOptions, useQuery } from '@tanstack/react-query'
import type { ReleaseNotes } from '../types/deployCompare'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const releaseNotesKeys = {
  notes: (appName: string, from: string, to: string | undefined) =>
    [
      ...appKeys.detail(appName),
      'deploys',
      'release-notes',
      from,
      to ?? 'latest',
    ] as const,
}

export async function fetchReleaseNotes(
  appName: string,
  from: string,
  to: string | undefined,
): Promise<ReleaseNotes> {
  const params = new URLSearchParams({ from })
  if (to) {
    params.set('to', to)
  }
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploys/release-notes?${params.toString()}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch release notes failed: ${res.status}`),
    )
  }
  return (await res.json()) as ReleaseNotes
}

export function useReleaseNotes(
  appName: string,
  from: string,
  to: string | undefined,
  enabled: boolean,
) {
  return useQuery(
    queryOptions({
      queryKey: releaseNotesKeys.notes(appName, from, to),
      queryFn: () => fetchReleaseNotes(appName, from, to),
      enabled,
    }),
  )
}
