// /api/v1/apps/{name}/build-hooks (internal/api/build_hooks.go): secret incoming URLs that build and deploy an app.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface BuildHook {
  id: string
  name: string
  branch?: string
  created_at: string
  last_triggered_at?: string
  last_result?: string
  trigger_count: number
  /** Set only on create and rotate, the one time the secret URL is visible. */
  path?: string
  token?: string
}

export interface CreateBuildHookInput {
  name: string
  branch: string
}

export const buildHookKeys = {
  list: (appName: string) =>
    [...appKeys.detail(appName), 'build-hooks'] as const,
}

function base(appName: string) {
  return `/api/v1/apps/${encodeURIComponent(appName)}/build-hooks`
}

async function send(
  url: string,
  init: RequestInit | undefined,
  failure: string,
): Promise<Response> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${failure}: ${res.status}`),
    )
  }
  return res
}

export function buildHooksQueryOptions(appName: string) {
  return queryOptions({
    queryKey: buildHookKeys.list(appName),
    queryFn: async () =>
      (await (
        await send(base(appName), undefined, 'fetch build hooks failed')
      ).json()) as BuildHook[],
  })
}

export function useBuildHooks(appName: string) {
  return useSuspenseQuery(buildHooksQueryOptions(appName))
}

function useInvalidateHooks(appName: string) {
  const queryClient = useQueryClient()
  return () =>
    queryClient.invalidateQueries({ queryKey: buildHookKeys.list(appName) })
}

export function useCreateBuildHook(appName: string) {
  const invalidate = useInvalidateHooks(appName)
  return useMutation<BuildHook, ApiError, CreateBuildHookInput>({
    mutationFn: async (input) =>
      (await (
        await send(
          base(appName),
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
          },
          'create build hook failed',
        )
      ).json()) as BuildHook,
    onSuccess: () => void invalidate(),
  })
}

export function useRotateBuildHook(appName: string) {
  const invalidate = useInvalidateHooks(appName)
  return useMutation<BuildHook, ApiError, string>({
    mutationFn: async (id) =>
      (await (
        await send(
          `${base(appName)}/${encodeURIComponent(id)}/rotate`,
          { method: 'POST' },
          'rotate build hook failed',
        )
      ).json()) as BuildHook,
    onSuccess: () => void invalidate(),
  })
}

export function useRevokeBuildHook(appName: string) {
  const invalidate = useInvalidateHooks(appName)
  return useMutation<void, ApiError, string>({
    mutationFn: async (id) => {
      await send(
        `${base(appName)}/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'revoke build hook failed',
      )
    },
    onSuccess: () => void invalidate(),
  })
}
