// GET/PUT /api/v1/apps/{name}/deploy-freeze (internal/api/deploy_safety.go):
// recurring windows during which automatic deploys are held.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { deployAttemptKeys } from './deployAttempts'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DeploySafetyValues } from '../lib/imageDigest'

export interface FreezeWindow {
  id?: string
  cron: string
  duration: string
  timezone?: string
  reason?: string
  scope?: string
}

export interface DeployFreeze {
  windows: FreezeWindow[]
  inherited?: FreezeWindow[]
  status: { frozen: boolean; until?: string; reason?: string }
}

export const deployFreezeKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'deploy-freeze'] as const,
}

function freezeURL(appName: string) {
  return `/api/v1/apps/${encodeURIComponent(appName)}/deploy-freeze`
}

async function fetchDeployFreeze(appName: string): Promise<DeployFreeze> {
  const res = await fetch(freezeURL(appName))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch deploy freeze failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeployFreeze
}

export function deployFreezeQueryOptions(appName: string) {
  return queryOptions({
    queryKey: deployFreezeKeys.detail(appName),
    queryFn: () => fetchDeployFreeze(appName),
    // A window can open or close while the page is open.
    refetchInterval: 60_000,
  })
}

// Optional: a control plane without freeze support answers 501, which must
// not break the pages that merely show the frozen state.
export function useDeployFreezeOptional(appName: string) {
  return useQuery({ ...deployFreezeQueryOptions(appName), retry: false })
}

async function putDeployFreeze(
  appName: string,
  windows: FreezeWindow[],
): Promise<DeployFreeze> {
  const res = await fetch(freezeURL(appName), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ windows }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `save deploy freeze failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeployFreeze
}

export function useSetDeployFreeze(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<DeployFreeze, ApiError, FreezeWindow[]>({
    mutationFn: (windows) => putDeployFreeze(appName, windows),
    onSuccess: (data) => {
      queryClient.setQueryData(deployFreezeKeys.detail(appName), data)
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(appName),
      })
    },
  })
}

// useFreezeBlocksDeploy reports whether a manual deploy still needs an
// override reason before it can be submitted.
export function useFreezeBlocksDeploy(
  appName: string,
  values: DeploySafetyValues,
): boolean {
  const freeze = useDeployFreezeOptional(appName)
  return (
    freeze.data?.status.frozen === true && values.overrideReason.trim() === ''
  )
}
