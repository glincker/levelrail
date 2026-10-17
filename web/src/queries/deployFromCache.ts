import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { ApiError } from '../lib/apiError'
import { appKeys } from './apps'
import { postJson } from './deployControl'

// Matches internal/api's triggerBuildResponse.
export interface DeployFromCacheResult {
  id?: string
  status?: string
}

/** useDeployFromCache rebuilds an app from the control plane's source cache, no git host needed. */
export function useDeployFromCache(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<DeployFromCacheResult, ApiError, void>({
    mutationFn: () =>
      postJson<DeployFromCacheResult>(
        `/api/v1/apps/${encodeURIComponent(appName)}/deploys/from-cache`,
        {},
        'deploy from cache',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: appKeys.detail(appName) }),
  })
}
