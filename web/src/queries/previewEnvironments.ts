// Query-key factory and fetchers for preview environments per pull
// request (internal/api/preview_environments_handlers.go): GET
// .../previews (list), POST .../previews/{number}/teardown (manual
// teardown), PUT .../preview-settings (the opt-in toggle, stored on the
// app's own git source, GitSource.PreviewEnabled).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { PreviewEnvironment } from '../types/previewEnvironment'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { gitSourceKeys } from './gitSources'
import type { GitSourceResource } from '../types/gitSource'

export const previewEnvironmentKeys = {
  all: ['preview-environments'] as const,
  list: (appName: string) => [...previewEnvironmentKeys.all, 'list', appName] as const,
}

export async function fetchPreviewEnvironments(appName: string): Promise<PreviewEnvironment[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/previews`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch preview environments failed: ${res.status}`),
    )
  }
  return (await res.json()) as PreviewEnvironment[]
}

// Polls while any preview is still 'deploying', the same conditional
// shape queries/deployAttempts.ts already uses for a running attempt:
// a preview transitions deploying -> active/failed from a webhook
// delivery this tab has no other way to learn about.
const DEPLOYING_PREVIEW_POLL_INTERVAL_MS = 3_000

export function previewEnvironmentsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: previewEnvironmentKeys.list(appName),
    queryFn: () => fetchPreviewEnvironments(appName),
    refetchInterval: (query) =>
      query.state.data?.some((p) => p.status === 'deploying')
        ? DEPLOYING_PREVIEW_POLL_INTERVAL_MS
        : false,
  })
}

export function usePreviewEnvironments(appName: string) {
  return useQuery(previewEnvironmentsQueryOptions(appName))
}

export async function teardownPreviewEnvironment(appName: string, prNumber: number): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/previews/${prNumber}/teardown`,
    { method: 'POST' },
  )
  if (!res.ok && res.status !== 207) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `teardown preview failed: ${res.status}`),
    )
  }
}

export function useTeardownPreviewEnvironment(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, number>({
    mutationFn: (prNumber) => teardownPreviewEnvironment(appName, prNumber),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: previewEnvironmentKeys.list(appName) })
    },
  })
}

export async function setPreviewEnabled(appName: string, enabled: boolean): Promise<{ enabled: boolean }> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/preview-settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  })
  if (res.status === 404) {
    throw new ApiError(404, 'Connect a git source before enabling preview environments.')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set preview enabled failed: ${res.status}`),
    )
  }
  return (await res.json()) as { enabled: boolean }
}

export function useSetPreviewEnabled(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<{ enabled: boolean }, ApiError, boolean>({
    mutationFn: (enabled) => setPreviewEnabled(appName, enabled),
    onSuccess: (result) => {
      queryClient.setQueryData<GitSourceResource | undefined>(
        gitSourceKeys.detail(appName),
        (current) => (current ? { ...current, preview_enabled: result.enabled } : current),
      )
      if (result.enabled) {
        void queryClient.invalidateQueries({ queryKey: previewEnvironmentKeys.list(appName) })
      }
    },
  })
}

// sweepStalePreviewEnvironments/useSweepStalePreviewEnvironments:
// POST /api/v1/previews/sweep, the manual trigger for the TTL fallback
// that already runs automatically in the background
// (internal/api/preview_environments_sweep.go), tearing down any preview
// whose pull-request-closed webhook never arrived. Cross-app, so success
// invalidates every app's own preview list, not just one.
export async function sweepStalePreviewEnvironments(): Promise<{ swept: number }> {
  const res = await fetch('/api/v1/previews/sweep', { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `sweep preview environments failed: ${res.status}`),
    )
  }
  return (await res.json()) as { swept: number }
}

export function useSweepStalePreviewEnvironments() {
  const queryClient = useQueryClient()
  return useMutation<{ swept: number }, ApiError, void>({
    mutationFn: () => sweepStalePreviewEnvironments(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: previewEnvironmentKeys.all })
    },
  })
}
