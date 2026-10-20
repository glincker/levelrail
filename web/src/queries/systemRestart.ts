import { useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { beginRestart } from '../lib/restartTracker'

// Matches internal/api's systemRestartResource.
export interface SystemRestartInfo {
  boot_id: string
  supported: boolean
  pending: boolean
  reason?: string
  hint?: string
}

const URL = '/api/v1/system/restart'

export async function fetchSystemRestart(
  signal?: AbortSignal,
): Promise<SystemRestartInfo> {
  const res = await fetch(URL, { signal, cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch restart info failed: ${res.status}`),
    )
  }
  return (await res.json()) as SystemRestartInfo
}

export function useSystemRestartInfo() {
  return useQuery({
    queryKey: ['system-restart'],
    queryFn: ({ signal }) => fetchSystemRestart(signal),
    staleTime: 30_000,
  })
}

export function useRestartControlPlane() {
  return useMutation({
    mutationFn: async (): Promise<SystemRestartInfo> => {
      const res = await fetch(URL, { method: 'POST' })
      if (!res.ok && res.status !== 202) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `restart failed: ${res.status}`),
        )
      }
      return (await res.json()) as SystemRestartInfo
    },
    onSuccess: (info) => beginRestart(info.boot_id),
  })
}
