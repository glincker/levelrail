// GET/PUT /api/v1/apps/{name}/auto-update and POST .../auto-update/check
// (internal/api/image_auto_update.go): redeploy when the image tag moves.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface ImageAutoUpdate {
  enabled: boolean
  last_checked_at?: string
  last_result?: string
}

export const imageAutoUpdateKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'auto-update'] as const,
}

function url(appName: string, suffix = '') {
  return `/api/v1/apps/${encodeURIComponent(appName)}/auto-update${suffix}`
}

async function request(
  appName: string,
  init: RequestInit | undefined,
  suffix: string,
  failure: string,
): Promise<ImageAutoUpdate> {
  const res = await fetch(url(appName, suffix), init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${failure}: ${res.status}`),
    )
  }
  return (await res.json()) as ImageAutoUpdate
}

export function imageAutoUpdateQueryOptions(appName: string) {
  return queryOptions({
    queryKey: imageAutoUpdateKeys.detail(appName),
    queryFn: () =>
      request(appName, undefined, '', 'fetch image auto-update failed'),
  })
}

export function useImageAutoUpdate(appName: string) {
  return useSuspenseQuery(imageAutoUpdateQueryOptions(appName))
}

export function useSetImageAutoUpdate(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<ImageAutoUpdate, ApiError, boolean>({
    mutationFn: (enabled) =>
      request(
        appName,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled }),
        },
        '',
        'set image auto-update failed',
      ),
    onSuccess: (result) => {
      queryClient.setQueryData(imageAutoUpdateKeys.detail(appName), result)
    },
  })
}

export function useCheckImageUpdate(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<ImageAutoUpdate, ApiError, void>({
    mutationFn: () =>
      request(
        appName,
        { method: 'POST' },
        '/check',
        'image update check failed',
      ),
    onSuccess: (result) => {
      queryClient.setQueryData(imageAutoUpdateKeys.detail(appName), result)
      void queryClient.invalidateQueries({
        queryKey: appKeys.detail(appName),
        exact: true,
      })
    },
  })
}
