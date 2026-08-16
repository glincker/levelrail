// Query-key factory and fetchers for the per-app git source resource,
// GET/PUT/DELETE /api/v1/apps/{name}/git-source
// (internal/api/git_sources.go), following the same shared-queryOptions
// pattern queries/databases.ts already establishes.
//
// fetchGitSource throws ApiError(404) when nothing is connected yet, the
// same "every fetcher throws on a non-ok response, callers narrow on
// error.status" convention queries/databases.ts/queries/nodes.ts already
// use: unlike those two, though, 404 here is this resource's normal,
// common steady state (most apps have no git source connected), not an
// exceptional one, so GitSourceCard.tsx uses a plain useQuery (not
// useSuspenseQuery) and narrows on error.status === 404 itself to render
// its "not connected" empty state, rather than letting it fall through
// to a route error boundary.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { GitSourceResource, SetGitSourceRequest } from '../types/gitSource'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const gitSourceKeys = {
  all: ['git-sources'] as const,
  detail: (name: string) => [...gitSourceKeys.all, 'detail', name] as const,
}

export async function fetchGitSource(name: string): Promise<GitSourceResource> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/git-source`)
  if (res.status === 404) {
    throw new ApiError(404, 'no git source connected for this app')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch git source failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourceResource
}

export function gitSourceQueryOptions(name: string) {
  return queryOptions({
    queryKey: gitSourceKeys.detail(name),
    queryFn: () => fetchGitSource(name),
    // A 404 (not connected yet) is the common case, not a transient
    // failure: retrying it would just burn requests for no benefit, the
    // same reasoning every 404-as-empty-state query in this codebase
    // skips retry for.
    retry: (failureCount, error) => {
      if (error instanceof ApiError && error.status === 404) {
        return false
      }
      return failureCount < 2
    },
  })
}

export function useGitSource(name: string) {
  return useQuery(gitSourceQueryOptions(name))
}

// PUT /api/v1/apps/{name}/git-source: connects a repo the first time
// it's called for an app (201, response includes a one-time
// webhook_secret), or edits the connection on every call after (200, no
// webhook_secret). 501 means no master key is configured on this control
// plane, the same "server configuration gap, not a bad submission"
// shape queries/backupTargets.ts's own createBackupTarget already
// documents for an identical case.
export async function setGitSource(
  name: string,
  req: SetGitSourceRequest,
): Promise<GitSourceResource> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/git-source`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Git sources require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `connect git source failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourceResource
}

export function useSetGitSource(name: string) {
  const queryClient = useQueryClient()
  return useMutation<GitSourceResource, ApiError, SetGitSourceRequest>({
    mutationFn: (req) => setGitSource(name, req),
    onSuccess: (resource) => {
      queryClient.setQueryData(gitSourceKeys.detail(name), resource)
    },
  })
}

// DELETE /api/v1/apps/{name}/git-source. 204 on success, no body.
export async function deleteGitSource(name: string): Promise<void> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/git-source`, {
    method: 'DELETE',
  })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `disconnect git source failed: ${res.status}`),
  )
}

export function useDeleteGitSource(name: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: () => deleteGitSource(name),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: gitSourceKeys.detail(name) })
    },
  })
}
