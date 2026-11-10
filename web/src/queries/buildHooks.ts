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
  /** Source ranges that may call the hook; empty allows any address. */
  allowed_cidrs?: string[]
  /** Latest authenticated call, including refused ones. */
  last_used_at?: string
  /** Latest refused call, kept apart from last_result. */
  last_refusal?: string
  /** Set only on create and rotate, the one time the secret URL is visible. */
  path?: string
  token?: string
}

export interface CreateBuildHookInput {
  name: string
  branch: string
  allowed_cidrs?: string[]
}

/** The server refuses a missing or empty list; clearing must say allow_any. */
export type UpdateBuildHookAllowlistInput =
  { id: string; allowed_cidrs: string[] } | { id: string; allow_any: true }

/** splitCidrs turns a comma or space separated field into entries; the server validates and canonicalizes them. */
export function splitCidrs(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((part) => part.trim())
    .filter(Boolean)
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

export function useUpdateBuildHookAllowlist(appName: string) {
  const invalidate = useInvalidateHooks(appName)
  return useMutation<BuildHook, ApiError, UpdateBuildHookAllowlistInput>({
    mutationFn: async ({ id, ...body }) =>
      (await (
        await send(
          `${base(appName)}/${encodeURIComponent(id)}`,
          {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
          },
          'update build hook failed',
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
