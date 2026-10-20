// "What changed" contract (GET /apps/{name}/changes, internal/api/app_changes.go):
// deploys, config, env key names and scaling in the window before `until`.

import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type AppChangeKind =
  | 'deploy'
  | 'rollback'
  | 'config'
  | 'env'
  | 'secret'
  | 'domain'
  | 'scale'
  | 'loadbalancer'
  | 'freeze'
  | 'maintenance'
  | 'lifecycle'

export interface AppChange {
  at: string
  kind: AppChangeKind
  actor: string
  title: string
  detail?: string
  keys?: string[]
  ref?: string
  likely_cause?: boolean
}

export interface AppChanges {
  app: string
  since: string
  until: string
  window_seconds: number
  changes: AppChange[]
  total: number
  truncated?: boolean
}

export const appChangesKeys = {
  one: (app: string, until: string) =>
    ['app-changes', app, until || 'now'] as const,
}

const NOW_REFETCH_MS = 30_000

export async function fetchAppChanges(
  app: string,
  until: string,
): Promise<AppChanges | null> {
  const qs = until ? `?until=${encodeURIComponent(until)}` : ''
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(app)}/changes${qs}`,
  )
  if (res.status === 404 || res.status === 501) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch changes failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppChanges
}

// until is an RFC 3339 instant (an alert's firing time); empty means now,
// which also refreshes on an interval.
export function useAppChanges(app: string, until: string, enabled = true) {
  return useQuery({
    queryKey: appChangesKeys.one(app, until),
    queryFn: () => fetchAppChanges(app, until),
    enabled,
    retry: false,
    staleTime: until ? Infinity : NOW_REFETCH_MS,
    refetchInterval: until ? false : NOW_REFETCH_MS,
  })
}
