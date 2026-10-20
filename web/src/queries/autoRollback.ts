// GET/PUT /api/v1/apps/{name}/auto-rollback (internal/api/deploys.go):
// whether internal/alerting.MaybeAutoRollback rolls this app back
// automatically the next time a kind=crashloop alert rule fires for it.
// Off by default, the same opt-in shape queries/previewEnvironments.ts's
// preview-settings toggle already establishes.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AutoRollbackSetting {
  enabled: boolean
}

export const autoRollbackKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'auto-rollback'] as const,
}

async function fetchAutoRollback(
  appName: string,
): Promise<AutoRollbackSetting> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/auto-rollback`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch auto-rollback setting failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AutoRollbackSetting
}

export function autoRollbackQueryOptions(appName: string) {
  return queryOptions({
    queryKey: autoRollbackKeys.detail(appName),
    queryFn: () => fetchAutoRollback(appName),
  })
}

export function useAutoRollback(appName: string) {
  return useSuspenseQuery(autoRollbackQueryOptions(appName))
}

async function putAutoRollback(
  appName: string,
  enabled: boolean,
): Promise<AutoRollbackSetting> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/auto-rollback`,
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
        `set auto-rollback setting failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AutoRollbackSetting
}

export function useSetAutoRollback(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<AutoRollbackSetting, ApiError, boolean>({
    mutationFn: (enabled) => putAutoRollback(appName, enabled),
    onSuccess: (result) => {
      queryClient.setQueryData(autoRollbackKeys.detail(appName), result)
    },
  })
}
