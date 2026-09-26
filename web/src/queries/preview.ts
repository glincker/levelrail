// Fetchers and hooks for the deploy preview screenshot routes
// (internal/api/preview.go). Non-suspense on purpose: a missing or
// unconfigured preview service (501) must never block the page it sits on.

import { useCallback, useEffect, useRef } from 'react'
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  PreviewPruneResult,
  PreviewRecord,
  PreviewSettingsInput,
  PreviewStatus,
} from '../types/preview'
import { appKeys } from './apps'
import { deployAttemptKeys } from './deployAttempts'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const previewKeys = {
  status: (appName: string) => [...appKeys.detail(appName), 'preview'] as const,
  history: (appName: string) =>
    [...appKeys.detail(appName), 'preview', 'history'] as const,
}

const CAPTURE_POLL_INTERVAL_MS = 3_000
const HISTORY_STALE_MS = 15_000

function previewUrl(appName: string, suffix = ''): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/preview${suffix}`
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function previewStatusQueryOptions(appName: string) {
  return queryOptions({
    queryKey: previewKeys.status(appName),
    queryFn: () =>
      request<PreviewStatus>(previewUrl(appName), undefined, 'fetch preview'),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.capturing ? CAPTURE_POLL_INTERVAL_MS : false,
  })
}

export function usePreviewStatus(appName: string) {
  return useQuery(previewStatusQueryOptions(appName))
}

export function previewHistoryQueryOptions(appName: string) {
  return queryOptions({
    queryKey: previewKeys.history(appName),
    queryFn: async () =>
      (await request<PreviewRecord[] | null>(
        previewUrl(appName, '/history'),
        undefined,
        'fetch preview history',
      )) ?? [],
    retry: false,
    staleTime: HISTORY_STALE_MS,
  })
}

export function usePreviewHistory(appName: string) {
  return useQuery(previewHistoryQueryOptions(appName))
}

function useInvalidatePreview(appName: string) {
  const queryClient = useQueryClient()
  return useCallback(
    () =>
      Promise.all([
        queryClient.invalidateQueries({
          queryKey: previewKeys.status(appName),
        }),
        queryClient.invalidateQueries({
          queryKey: previewKeys.history(appName),
        }),
        queryClient.invalidateQueries({
          queryKey: deployAttemptKeys.list(appName),
        }),
      ]),
    [queryClient, appName],
  )
}

export function useSetPreview(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<PreviewStatus, ApiError, PreviewSettingsInput>({
    mutationFn: (input) =>
      request<PreviewStatus>(
        previewUrl(appName),
        jsonInit('PUT', input),
        'save preview settings',
      ),
    onSuccess: (status) => {
      queryClient.setQueryData(previewKeys.status(appName), status)
    },
  })
}

export function useCapturePreview(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<PreviewStatus, ApiError, void>({
    mutationFn: () =>
      request<PreviewStatus>(
        previewUrl(appName, '/capture'),
        jsonInit('POST'),
        'queue preview capture',
      ),
    onSuccess: (status) => {
      queryClient.setQueryData(previewKeys.status(appName), status)
    },
  })
}

export function usePrunePreview(appName: string) {
  const invalidate = useInvalidatePreview(appName)
  return useMutation<PreviewPruneResult, ApiError, void>({
    mutationFn: () =>
      request<PreviewPruneResult>(
        previewUrl(appName, '/prune'),
        jsonInit('POST', {}),
        'prune previews',
      ),
    onSuccess: () => invalidate(),
  })
}

// Refetches history and deploy history once a capture the caller started
// has finished (status.capturing flips from true back to false).
export function useRefreshWhenCaptureEnds(
  appName: string,
  capturing: boolean | undefined,
) {
  const invalidate = useInvalidatePreview(appName)
  const wasCapturing = useRef(false)
  useEffect(() => {
    if (wasCapturing.current && capturing === false) void invalidate()
    wasCapturing.current = capturing === true
  }, [capturing, invalidate])
}
