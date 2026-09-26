// Activity timeline and pending-changes contract (GET /apps/{name}/timeline,
// GET /apps/{name}/pending-changes, POST /apps/{name}/apply-pending). Both
// reads resolve to null on 404 or 501 so the page works before the backend
// lands.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type TimelineKind =
  | 'deploy'
  | 'restart'
  | 'env_change'
  | 'secret_change'
  | 'config_change'
  | 'rollback'
  | 'scale'
  | 'freeze_override'
  | 'suspend'
  | 'resume'

export type TimelineStatus =
  'succeeded' | 'failed' | 'in_progress' | 'pending' | 'info'

export interface TimelineEntry {
  id: string
  at: string
  kind: TimelineKind
  status: TimelineStatus
  actor: string
  title: string
  detail?: string
  ref?: { type: 'deploy_attempt'; id: string }
}

export interface PendingChanges {
  pending: boolean
  changes: {
    kind: 'env' | 'secret' | 'config'
    keys?: string[]
    since: string
  }[]
  apply_action: 'restart' | 'redeploy'
}

const TIMELINE_LIMIT = 8
const TIMELINE_REFETCH_MS = 20_000

export const timelineKeys = {
  list: (appName: string) => [...appKeys.detail(appName), 'timeline'] as const,
  pending: (appName: string) =>
    [...appKeys.detail(appName), 'pending-changes'] as const,
}

function isMissing(status: number): boolean {
  return status === 404 || status === 501
}

export async function fetchTimeline(
  appName: string,
): Promise<TimelineEntry[] | null> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/timeline?limit=${TIMELINE_LIMIT}`,
  )
  if (isMissing(res.status)) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch timeline failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { items?: TimelineEntry[] | null }
  return body.items ?? []
}

export async function fetchPendingChanges(
  appName: string,
): Promise<PendingChanges | null> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/pending-changes`,
  )
  if (isMissing(res.status)) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch pending changes failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as PendingChanges
}

/** Resolves to null when the backend has no timeline endpoint yet. */
export function useAppTimeline(appName: string) {
  return useQuery({
    queryKey: timelineKeys.list(appName),
    queryFn: () => fetchTimeline(appName),
    refetchInterval: TIMELINE_REFETCH_MS,
    retry: false,
  })
}

export function usePendingChanges(appName: string) {
  return useQuery({
    queryKey: timelineKeys.pending(appName),
    queryFn: () => fetchPendingChanges(appName),
    refetchInterval: TIMELINE_REFETCH_MS,
    retry: false,
  })
}

export async function applyPending(appName: string): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/apply-pending`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `apply pending changes failed: ${res.status}`,
      ),
    )
  }
}

export function useApplyPending(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => applyPending(appName),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(appName) })
    },
  })
}
