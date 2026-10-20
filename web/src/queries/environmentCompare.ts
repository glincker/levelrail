// Query-key factory and fetcher for GET
// /api/v1/projects/{id}/environments/compare
// (internal/api/environment_compare.go's handleCompareEnvironmentEnv).
// Kept in its own module, separate from queries/environments.ts, the
// same "genuinely different resource" reasoning queries/deployCompare.ts
// already gives for its own sibling file.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import type { EnvironmentCompare } from '../types/environmentCompare'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const environmentCompareKeys = {
  compare: (projectId: string, a: string, b: string) =>
    ['environments', 'compare', projectId, a, b] as const,
}

export async function fetchEnvironmentCompare(
  projectId: string,
  a: string,
  b: string,
): Promise<EnvironmentCompare> {
  const params = new URLSearchParams({ a, b })
  const res = await fetch(
    `/api/v1/projects/${encodeURIComponent(projectId)}/environments/compare?${params.toString()}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch environment comparison failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as EnvironmentCompare
}

export function environmentCompareQueryOptions(
  projectId: string,
  a: string,
  b: string,
) {
  return queryOptions({
    queryKey: environmentCompareKeys.compare(projectId, a, b),
    queryFn: () => fetchEnvironmentCompare(projectId, a, b),
    enabled: projectId !== '' && a !== '' && b !== '',
  })
}

export function useEnvironmentCompare(projectId: string, a: string, b: string) {
  return useSuspenseQuery(environmentCompareQueryOptions(projectId, a, b))
}
