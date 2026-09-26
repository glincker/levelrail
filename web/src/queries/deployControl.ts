// Cancel, digest-pinned rollback and the cancel-superseded setting:
// POST /api/v1/apps/{name}/deploys/{deployId}/cancel and .../rollback,
// GET/PUT /api/v1/apps/{name}/cancel-superseded (internal/api/deploy_cancel.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { DeployAttempt } from '../types/deployAttempt'
import { appKeys } from './apps'
import { deployAttemptKeys } from './deployAttempts'
import { applyTriggerDeployResult } from './deploys'
import type { TriggerDeployResult } from './deploys'
import { ApiError, readErrorMessage } from '../lib/apiError'

async function postJson<T>(
  url: string,
  body: unknown,
  what: string,
): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useCancelDeploy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<DeployAttempt, ApiError, string>({
    mutationFn: (deployId) =>
      postJson<DeployAttempt>(
        `/api/v1/apps/${encodeURIComponent(appName)}/deploys/${encodeURIComponent(deployId)}/cancel`,
        {},
        'cancel deploy',
      ),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(appName),
      })
    },
  })
}

export interface RollbackToInput {
  deployId: string
  confirm?: boolean
  overrideFreeze?: boolean
  overrideReason?: string
}

export function useRollbackToDeploy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<TriggerDeployResult, ApiError, RollbackToInput>({
    mutationFn: (input) =>
      postJson<TriggerDeployResult>(
        `/api/v1/apps/${encodeURIComponent(appName)}/deploys/${encodeURIComponent(input.deployId)}/rollback`,
        {
          confirm: input.confirm ?? false,
          override_freeze: input.overrideFreeze ?? false,
          override_reason: input.overrideReason ?? '',
        },
        'roll back deploy',
      ),
    onSuccess: (result) =>
      applyTriggerDeployResult(queryClient, appName, result),
  })
}

export interface CancelSupersededSetting {
  enabled: boolean
}

export const cancelSupersededKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'cancel-superseded'] as const,
}

export function cancelSupersededQueryOptions(appName: string) {
  return queryOptions({
    queryKey: cancelSupersededKeys.detail(appName),
    queryFn: async (): Promise<CancelSupersededSetting> => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/cancel-superseded`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch cancel-superseded setting failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as CancelSupersededSetting
    },
  })
}

export function useCancelSuperseded(appName: string) {
  return useQuery(cancelSupersededQueryOptions(appName))
}

export function useSetCancelSuperseded(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<CancelSupersededSetting, ApiError, boolean>({
    mutationFn: async (enabled) => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/cancel-superseded`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled }),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `set cancel-superseded setting failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as CancelSupersededSetting
    },
    onSuccess: (result) => {
      queryClient.setQueryData(cancelSupersededKeys.detail(appName), result)
    },
  })
}
