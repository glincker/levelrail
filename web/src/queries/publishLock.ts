// GET/PUT/DELETE /api/v1/apps/{name}/publish-lock and
// POST .../deploys/{id}/publish and .../rollback-lock (internal/api/publish_lock.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { deployAttemptKeys } from './deployAttempts'
import { postJson } from './deployControl'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface PublishLock {
  locked: boolean
  reason?: string
  locked_by?: string
  locked_at?: string
  pinned_image?: string
  held_builds: string[]
}

export const publishLockKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'publish-lock'] as const,
}

function lockURL(appName: string) {
  return `/api/v1/apps/${encodeURIComponent(appName)}/publish-lock`
}

async function sendLock(
  appName: string,
  method: 'PUT' | 'DELETE',
  body?: { reason: string },
): Promise<PublishLock> {
  const res = await fetch(lockURL(appName), {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `publish lock failed: ${res.status}`),
    )
  }
  return (await res.json()) as PublishLock
}

export function publishLockQueryOptions(appName: string) {
  return queryOptions({
    queryKey: publishLockKeys.detail(appName),
    queryFn: async (): Promise<PublishLock> => {
      const res = await fetch(lockURL(appName))
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch publish lock failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as PublishLock
    },
    refetchInterval: 30_000,
    retry: false,
  })
}

// A control plane without publish locks answers 501; callers then show nothing.
export function usePublishLock(appName: string) {
  return useQuery(publishLockQueryOptions(appName))
}

export function usePublishLockActions(appName: string) {
  const queryClient = useQueryClient()
  const refresh = () => {
    void queryClient.invalidateQueries({
      queryKey: publishLockKeys.detail(appName),
    })
    void queryClient.invalidateQueries({
      queryKey: deployAttemptKeys.list(appName),
    })
    void queryClient.invalidateQueries({ queryKey: appKeys.detail(appName) })
  }
  const lock = useMutation<PublishLock, ApiError, string>({
    mutationFn: (reason) => sendLock(appName, 'PUT', { reason }),
    onSuccess: refresh,
  })
  const unlock = useMutation<PublishLock, ApiError, void>({
    mutationFn: () => sendLock(appName, 'DELETE'),
    onSuccess: refresh,
  })
  const publish = useMutation<unknown, ApiError, string>({
    mutationFn: (deployId) =>
      postJson(
        `/api/v1/apps/${encodeURIComponent(appName)}/deploys/${encodeURIComponent(deployId)}/publish`,
        {},
        'publish deploy',
      ),
    onSuccess: refresh,
  })
  const rollbackAndLock = useMutation<unknown, ApiError, string>({
    mutationFn: (deployId) =>
      postJson(
        `/api/v1/apps/${encodeURIComponent(appName)}/deploys/${encodeURIComponent(deployId)}/rollback-lock`,
        {},
        'roll back and lock',
      ),
    onSuccess: refresh,
  })
  return { lock, unlock, publish, rollbackAndLock }
}
