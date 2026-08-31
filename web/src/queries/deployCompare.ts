// Query-key factory and fetcher for
// GET /api/v1/apps/{name}/deploys/compare (internal/api/deploy_compare.go's
// handleCompareDeploys). Kept in its own module, separate from
// queries/deployAttempts.ts, the same "genuinely different resource"
// reasoning that file's own doc comment already gives for deploys.ts.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import type { DeployCompare } from '../types/deployCompare'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const deployCompareKeys = {
  compare: (appName: string, from: string, to: string | undefined) =>
    [...appKeys.detail(appName), 'deploys', 'compare', from, to ?? 'current'] as const,
}

export async function fetchDeployCompare(
  appName: string,
  from: string,
  to: string | undefined,
): Promise<DeployCompare> {
  const params = new URLSearchParams({ from })
  if (to) {
    params.set('to', to)
  }
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploys/compare?${params.toString()}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch deploy comparison failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeployCompare
}

export function deployCompareQueryOptions(
  appName: string,
  from: string,
  to: string | undefined,
) {
  return queryOptions({
    queryKey: deployCompareKeys.compare(appName, from, to),
    queryFn: () => fetchDeployCompare(appName, from, to),
  })
}

export function useDeployCompare(
  appName: string,
  from: string,
  to: string | undefined,
) {
  return useSuspenseQuery(deployCompareQueryOptions(appName, from, to))
}
