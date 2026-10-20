// GET/PUT /api/v1/apps/{name}/badge (internal/api/app_badge.go): the
// opt-in flag gating the public GET .../badge.svg route. Off by default,
// the opposite shape execAccess.ts's own opt-out toggle establishes:
// a badge exposes deploy status to anyone with the URL, so it must be
// an explicit per-app choice.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface BadgeSetting {
  enabled: boolean
}

export const badgeKeys = {
  detail: (appName: string) => [...appKeys.detail(appName), 'badge'] as const,
}

async function fetchBadgeSetting(appName: string): Promise<BadgeSetting> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/badge`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch badge setting failed: ${res.status}`),
    )
  }
  return (await res.json()) as BadgeSetting
}

export function badgeQueryOptions(appName: string) {
  return queryOptions({
    queryKey: badgeKeys.detail(appName),
    queryFn: () => fetchBadgeSetting(appName),
  })
}

export function useBadgeSetting(appName: string) {
  return useSuspenseQuery(badgeQueryOptions(appName))
}

async function putBadgeSetting(
  appName: string,
  enabled: boolean,
): Promise<BadgeSetting> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/badge`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set badge setting failed: ${res.status}`),
    )
  }
  return (await res.json()) as BadgeSetting
}

export function useSetBadgeSetting(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<BadgeSetting, ApiError, boolean>({
    mutationFn: (enabled) => putBadgeSetting(appName, enabled),
    onSuccess: (result) => {
      queryClient.setQueryData(badgeKeys.detail(appName), result)
    },
  })
}
