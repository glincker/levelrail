// POST /api/v1/apps/{name}/keep (internal/api/apps_keep.go): a one-click
// trial becomes a permanent app.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface KeepTrialResult {
  name: string
  kept: string[]
  kept_at: string
  already_kept: boolean
}

export async function keepTrialApp(name: string): Promise<KeepTrialResult> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/keep`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `keep app failed: ${res.status}`),
    )
  }
  return (await res.json()) as KeepTrialResult
}

export function useKeepTrialApp() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: keepTrialApp,
    onSuccess: (res) => {
      for (const name of [res.name, ...res.kept]) {
        void queryClient.invalidateQueries({ queryKey: appKeys.detail(name) })
      }
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
