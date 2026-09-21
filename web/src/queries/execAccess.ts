// GET/PUT /api/v1/apps/{name}/exec-access (internal/api/exec.go): whether
// POST .../exec and the interactive terminal (GET .../terminal) are even
// attempted for this app, independent of the caller's own IAM abilities.
// On by default, unlike queries/autoRollback.ts's own opt-in toggle:
// exec is available today with no equivalent gate, so this preserves
// existing behavior until an operator explicitly disables it.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface ExecAccessSetting {
  enabled: boolean
}

export const execAccessKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'exec-access'] as const,
}

async function fetchExecAccess(appName: string): Promise<ExecAccessSetting> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/exec-access`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch exec access setting failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ExecAccessSetting
}

export function execAccessQueryOptions(appName: string) {
  return queryOptions({
    queryKey: execAccessKeys.detail(appName),
    queryFn: () => fetchExecAccess(appName),
  })
}

export function useExecAccess(appName: string) {
  return useSuspenseQuery(execAccessQueryOptions(appName))
}

async function putExecAccess(
  appName: string,
  enabled: boolean,
): Promise<ExecAccessSetting> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/exec-access`,
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
        `set exec access setting failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ExecAccessSetting
}

export function useSetExecAccess(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<ExecAccessSetting, ApiError, boolean>({
    mutationFn: (enabled) => putExecAccess(appName, enabled),
    onSuccess: (result) => {
      queryClient.setQueryData(execAccessKeys.detail(appName), result)
    },
  })
}
