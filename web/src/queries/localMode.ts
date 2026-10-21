import { queryOptions, useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { beginRestart } from '../lib/restartTracker'
import { fetchSystemRestart } from './systemRestart'

// Matches internal/api's localModeResource.
export interface LocalModeInfo {
  enabled: boolean
  data_dir?: string
  reset_supported: boolean
  reset_hint?: string
}

const URL = '/api/v1/local-mode'

export const localModeKeys = {
  all: ['local-mode'] as const,
}

export async function fetchLocalMode(
  signal?: AbortSignal,
): Promise<LocalModeInfo> {
  const res = await fetch(URL, { signal })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch local mode failed: ${res.status}`),
    )
  }
  return (await res.json()) as LocalModeInfo
}

// staleTime: Infinity, since APP_LOCAL_MODE is fixed for a running process.
export function localModeQueryOptions() {
  return queryOptions({
    queryKey: localModeKeys.all,
    queryFn: ({ signal }) => fetchLocalMode(signal),
    staleTime: Infinity,
  })
}

export function useLocalMode() {
  return useQuery(localModeQueryOptions())
}

export function useResetLocalInstance() {
  return useMutation({
    mutationFn: async (): Promise<void> => {
      const before = await fetchSystemRestart()
      const res = await fetch(`${URL}/reset`, { method: 'POST' })
      if (!res.ok && res.status !== 202) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `reset failed: ${res.status}`),
        )
      }
      beginRestart(before.boot_id)
    },
  })
}
