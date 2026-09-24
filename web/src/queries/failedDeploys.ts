import { queryOptions } from '@tanstack/react-query'
import type { DeployAttempt } from '../types/deployAttempt'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface FailedDeploy extends DeployAttempt {
  last_good_image?: string
}

export const failedDeployKeys = {
  all: ['deploys', 'failed'] as const,
}

// GET /api/v1/deploys/failed: each app's latest attempt when it failed in
// the last 24 hours.
export async function fetchFailedDeploys(): Promise<FailedDeploy[]> {
  const res = await fetch('/api/v1/deploys/failed?since=24h')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch failed deploys failed: ${res.status}`),
    )
  }
  return ((await res.json()) as FailedDeploy[] | null) ?? []
}

export function failedDeploysQueryOptions() {
  return queryOptions({
    queryKey: failedDeployKeys.all,
    queryFn: fetchFailedDeploys,
  })
}
