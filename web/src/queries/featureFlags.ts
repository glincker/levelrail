// Query-key factory and fetchers for GET/POST/PUT/DELETE
// /api/v1/apps/{name}/flags[/{id}] (internal/api/feature_flags.go). Kept
// in its own module for the same reason queries/scheduledTasks.ts is: a
// genuinely different resource shape than AppDetail, nested under the
// same app name.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { FeatureFlag, FeatureFlagRequest } from '../types/featureFlags'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const featureFlagKeys = {
  all: (appName: string) => [...appKeys.detail(appName), 'flags'] as const,
  list: (appName: string) => [...featureFlagKeys.all(appName), 'list'] as const,
}

export async function fetchFeatureFlags(appName: string): Promise<FeatureFlag[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/flags`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch feature flags failed: ${res.status}`),
    )
  }
  return (await res.json()) as FeatureFlag[]
}

export function featureFlagListQueryOptions(appName: string) {
  return queryOptions({
    queryKey: featureFlagKeys.list(appName),
    queryFn: () => fetchFeatureFlags(appName),
  })
}

export function useFeatureFlags(appName: string) {
  return useQuery(featureFlagListQueryOptions(appName))
}

export async function createFeatureFlag(
  appName: string,
  req: FeatureFlagRequest,
): Promise<FeatureFlag> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/flags`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create feature flag failed: ${res.status}`),
    )
  }
  return (await res.json()) as FeatureFlag
}

export function useCreateFeatureFlag(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: FeatureFlagRequest) => createFeatureFlag(appName, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: featureFlagKeys.list(appName) })
    },
  })
}

export async function updateFeatureFlag(
  appName: string,
  id: string,
  req: FeatureFlagRequest,
): Promise<FeatureFlag> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/flags/${encodeURIComponent(id)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update feature flag failed: ${res.status}`),
    )
  }
  return (await res.json()) as FeatureFlag
}

export function useUpdateFeatureFlag(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: FeatureFlagRequest }) =>
      updateFeatureFlag(appName, id, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: featureFlagKeys.list(appName) })
    },
  })
}

export async function deleteFeatureFlag(appName: string, id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/flags/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete feature flag failed: ${res.status}`),
    )
  }
}

export function useDeleteFeatureFlag(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteFeatureFlag(appName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: featureFlagKeys.list(appName) })
    },
  })
}
