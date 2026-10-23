import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DeployEventComparison } from '../types/deployEvents'

export const deployEventKeys = {
  compare: (id: string, other: string) =>
    ['deploy-events', 'compare', id, other] as const,
}

async function fetchComparison(
  id: string,
  other: string,
): Promise<DeployEventComparison> {
  const res = await fetch(
    `/api/v1/deployments/${encodeURIComponent(id)}/timeline/compare?with=${encodeURIComponent(other)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `deploy timeline compare failed: ${String(res.status)}`,
      ),
    )
  }
  return (await res.json()) as DeployEventComparison
}

export function deployEventCompareOptions(id: string, other: string) {
  return queryOptions({
    queryKey: deployEventKeys.compare(id, other),
    queryFn: () => fetchComparison(id, other),
    staleTime: 30_000,
  })
}

export function useDeployEventComparison(id: string, other: string) {
  return useQuery({
    ...deployEventCompareOptions(id, other),
    enabled: other !== '',
  })
}
