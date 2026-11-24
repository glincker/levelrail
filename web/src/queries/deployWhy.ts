import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DeployWhy } from '../types/deployWhy'

export const deployWhyKeys = {
  one: (id: string) => ['deploy-why', id] as const,
}

async function fetchDeployWhy(id: string): Promise<DeployWhy> {
  const res = await fetch(`/api/v1/deploys/${encodeURIComponent(id)}/why`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `deploy why failed: ${String(res.status)}`),
    )
  }
  return (await res.json()) as DeployWhy
}

export function deployWhyOptions(id: string) {
  return queryOptions({
    queryKey: deployWhyKeys.one(id),
    queryFn: () => fetchDeployWhy(id),
    staleTime: 60_000,
  })
}

/** Loads only once the panel is opened, so a long history does not mine every failed row. */
export function useDeployWhy(id: string, enabled: boolean) {
  return useQuery({ ...deployWhyOptions(id), enabled })
}
