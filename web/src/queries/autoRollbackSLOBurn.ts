// GET/PUT /api/v1/apps/{name}/auto-rollback-slo-burn (internal/api/deploys.go):
// which mode internal/alerting.MaybeAutoRollbackOnSLOBurn acts in the next
// time a kind=slo_burn alert rule fires for this app. "off" by default, the
// same opt-in shape queries/autoRollback.ts's crashloop toggle already
// establishes, but a mode string rather than a bool since there are three
// distinct ways to react (auto, dry_run, pause_for_human) plus off.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type SLOBurnAutoRollbackMode =
  'off' | 'auto' | 'dry_run' | 'pause_for_human'

export interface AutoRollbackSLOBurnSetting {
  mode: SLOBurnAutoRollbackMode
}

export const autoRollbackSLOBurnKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'auto-rollback-slo-burn'] as const,
}

async function fetchAutoRollbackSLOBurn(
  appName: string,
): Promise<AutoRollbackSLOBurnSetting> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/auto-rollback-slo-burn`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch auto-rollback-slo-burn setting failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AutoRollbackSLOBurnSetting
}

export function autoRollbackSLOBurnQueryOptions(appName: string) {
  return queryOptions({
    queryKey: autoRollbackSLOBurnKeys.detail(appName),
    queryFn: () => fetchAutoRollbackSLOBurn(appName),
  })
}

export function useAutoRollbackSLOBurn(appName: string) {
  return useSuspenseQuery(autoRollbackSLOBurnQueryOptions(appName))
}

async function putAutoRollbackSLOBurn(
  appName: string,
  mode: SLOBurnAutoRollbackMode,
): Promise<AutoRollbackSLOBurnSetting> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/auto-rollback-slo-burn`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `set auto-rollback-slo-burn setting failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AutoRollbackSLOBurnSetting
}

export function useSetAutoRollbackSLOBurn(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<
    AutoRollbackSLOBurnSetting,
    ApiError,
    SLOBurnAutoRollbackMode
  >({
    mutationFn: (mode) => putAutoRollbackSLOBurn(appName, mode),
    onSuccess: (result) => {
      queryClient.setQueryData(autoRollbackSLOBurnKeys.detail(appName), result)
    },
  })
}
