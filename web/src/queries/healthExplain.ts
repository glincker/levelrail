// Query for GET /api/v1/apps/{name}/health-explain, the "why is this red or amber" line.
import { queryOptions, useQuery } from '@tanstack/react-query'
import type { AppHealthExplain } from '../types/healthExplain'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollMs } from '@/lib/pollTier'

export const healthExplainKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'health-explain'] as const,
}

export async function fetchAppHealthExplain(
  appName: string,
): Promise<AppHealthExplain> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/health-explain`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch health explain failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppHealthExplain
}

export function useAppHealthExplain(appName: string, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: healthExplainKeys.detail(appName),
      queryFn: () => fetchAppHealthExplain(appName),
      refetchInterval: pollMs('steady'),
      enabled,
    }),
  )
}
