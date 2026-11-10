// GET/PUT /api/v1/apps/{name}/compose-source and POST .../deploy
// (internal/api/compose_git_source.go). Disconnect reuses DELETE
// .../git-source, since both kinds live in the same git source record.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  GitSourceResource,
  GitSourceTriggerMode,
} from '../types/gitSource'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { deleteGitSource, gitSourceKeys } from './gitSources'
import type { ComposeDeployResponse } from './compose'
import { composeKeys } from './composeStack'

export interface SetComposeSourceRequest {
  repo_url: string
  branch?: string
  compose_path?: string
  token?: string
  trigger_mode?: GitSourceTriggerMode
}

export const composeSourceKeys = {
  all: ['compose-source'] as const,
  detail: (name: string) => [...composeSourceKeys.all, name] as const,
}

function path(name: string, suffix = ''): string {
  return `/api/v1/apps/${encodeURIComponent(name)}/compose-source${suffix}`
}

export async function fetchComposeSource(
  name: string,
): Promise<GitSourceResource> {
  const res = await fetch(path(name))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch compose source failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourceResource
}

export function composeSourceQueryOptions(name: string) {
  return queryOptions({
    queryKey: composeSourceKeys.detail(name),
    queryFn: () => fetchComposeSource(name),
    // 404 is the normal "not connected" state, never worth a retry.
    retry: (failureCount, error) =>
      !(error instanceof ApiError && error.status === 404) && failureCount < 2,
  })
}

export function useComposeSource(name: string) {
  return useQuery(composeSourceQueryOptions(name))
}

export async function setComposeSource(
  name: string,
  req: SetComposeSourceRequest,
): Promise<GitSourceResource> {
  const res = await fetch(path(name), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `connect compose source failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitSourceResource
}

// Strips the one-time webhook secret so it never sits in the query cache.
export function withoutSecret(r: GitSourceResource): GitSourceResource {
  const rest = { ...r }
  delete rest.webhook_secret
  return rest
}

export function useSetComposeSource(name: string) {
  const queryClient = useQueryClient()
  return useMutation<GitSourceResource, ApiError, SetComposeSourceRequest>({
    meta: { errorToast: false },
    mutationFn: (req) => setComposeSource(name, req),
    onSuccess: (resource) => {
      queryClient.setQueryData(
        composeSourceKeys.detail(name),
        withoutSecret(resource),
      )
    },
  })
}

export async function deployComposeSource(
  name: string,
): Promise<ComposeDeployResponse> {
  const res = await fetch(path(name, '/deploy'), { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `deploy compose source failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ComposeDeployResponse
}

export function useDeployComposeSource(name: string) {
  const queryClient = useQueryClient()
  return useMutation<ComposeDeployResponse, ApiError, void>({
    meta: { errorToast: false },
    mutationFn: () => deployComposeSource(name),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: composeKeys.all })
    },
  })
}

export function useDisconnectComposeSource(name: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    meta: { errorToast: false },
    mutationFn: () => deleteGitSource(name),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: composeSourceKeys.detail(name) })
      void queryClient.invalidateQueries({
        queryKey: gitSourceKeys.detail(name),
      })
    },
  })
}
