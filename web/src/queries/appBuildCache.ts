// Fetchers for /api/v1/apps/{name}/build-cache (internal/api/apps_build_cache.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AppBuildCache {
  cache_id: string
  size_bytes: number
  record_count: number
  in_use: boolean
  last_used_at?: string
}

export interface AppBuildCacheClearResult {
  cache_id: string
  freed_bytes: number
  removed_count: number
}

export const appBuildCacheKey = (app: string) =>
  ['apps', app, 'build-cache'] as const

const path = (app: string) =>
  `/api/v1/apps/${encodeURIComponent(app)}/build-cache`

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `build cache request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useAppBuildCache(app: string) {
  return useQuery({
    queryKey: appBuildCacheKey(app),
    queryFn: () => request<AppBuildCache>(path(app)),
    staleTime: 10_000,
    retry: false,
  })
}

export function useClearAppBuildCache(app: string) {
  const qc = useQueryClient()
  return useMutation<AppBuildCacheClearResult, Error>({
    mutationFn: () =>
      request<AppBuildCacheClearResult>(path(app), { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: appBuildCacheKey(app) })
    },
  })
}
