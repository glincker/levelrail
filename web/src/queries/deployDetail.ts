// GET /api/v1/apps/{name}/deploys/{deployId}: one attempt with its
// classified failure and, when its container stopped before serving, the
// captured exit and last output (internal/api/deploy_failure.go).

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { DeployAttempt } from '../types/deployAttempt'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const deployDetailKeys = {
  detail: (appName: string, deployId: string) =>
    [...appKeys.detail(appName), 'deploy', deployId] as const,
}

export async function fetchDeployDetail(
  appName: string,
  deployId: string,
): Promise<DeployAttempt> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploys/${encodeURIComponent(deployId)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch deploy failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeployAttempt
}

export function deployDetailQueryOptions(appName: string, deployId: string) {
  return queryOptions({
    queryKey: deployDetailKeys.detail(appName, deployId),
    queryFn: () => fetchDeployDetail(appName, deployId),
  })
}

export function useDeployDetail(
  appName: string,
  deployId: string,
  enabled: boolean,
) {
  return useQuery({ ...deployDetailQueryOptions(appName, deployId), enabled })
}
