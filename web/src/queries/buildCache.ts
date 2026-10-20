// Query-key factory, fetchers and mutations for the BuildKit remote build
// cache on a storage destination (internal/api/build_cache.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  BuildCacheClearResult,
  BuildCacheSetting,
  BuildCacheStats,
  SetBuildCacheRequest,
} from '../types/storage'

export const buildCacheKeys = {
  all: ['build-cache'] as const,
  settings: () => [...buildCacheKeys.all, 'settings'] as const,
  stats: (app: string) => [...buildCacheKeys.all, 'stats', app] as const,
}

async function requestJson<T>(
  input: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(input, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function buildCacheSettingsQueryOptions() {
  return queryOptions({
    queryKey: buildCacheKeys.settings(),
    queryFn: () =>
      requestJson<BuildCacheSetting[]>(
        '/api/v1/build-cache',
        undefined,
        'fetch build cache settings',
      ),
    retry: false,
  })
}

export function useBuildCacheSettings() {
  return useQuery(buildCacheSettingsQueryOptions())
}

// Lists the bucket, so callers enable it only for a configured app.
export function useBuildCacheStats(app: string, enabled: boolean) {
  return useQuery({
    queryKey: buildCacheKeys.stats(app),
    queryFn: () =>
      requestJson<BuildCacheStats>(
        `/api/v1/build-cache/stats?app=${encodeURIComponent(app)}`,
        undefined,
        'fetch build cache usage',
      ),
    enabled,
    retry: false,
  })
}

export function useSetBuildCache() {
  const queryClient = useQueryClient()
  return useMutation<BuildCacheSetting, ApiError, SetBuildCacheRequest>({
    mutationFn: (req) =>
      requestJson<BuildCacheSetting>(
        '/api/v1/build-cache',
        jsonInit('PUT', req),
        'save build cache',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: buildCacheKeys.all }),
  })
}

export function useRemoveBuildCache() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (app) => {
      const res = await fetch(
        `/api/v1/build-cache?app=${encodeURIComponent(app)}`,
        { method: 'DELETE' },
      )
      if (res.status !== 204) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `remove build cache failed: ${res.status}`,
          ),
        )
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: buildCacheKeys.all }),
  })
}

export function useClearBuildCache() {
  const queryClient = useQueryClient()
  return useMutation<BuildCacheClearResult, ApiError, string>({
    mutationFn: (app) =>
      requestJson<BuildCacheClearResult>(
        '/api/v1/build-cache/clear',
        jsonInit('POST', { app_name: app }),
        'clear build cache',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: buildCacheKeys.all }),
  })
}
