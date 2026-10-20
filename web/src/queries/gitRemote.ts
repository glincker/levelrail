// Push deploy remote of an app: GET/POST/PUT/DELETE /api/v1/apps/{name}/git-remote
// (internal/api/git_push_remote.go). The token comes back once, on POST.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { GitRemoteResource } from '../types/gitRemote'

export const gitRemoteKeys = {
  detail: (name: string) => ['git-remote', name] as const,
}

function path(name: string): string {
  return `/api/v1/apps/${encodeURIComponent(name)}/git-remote`
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  failure: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${failure}: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function gitRemoteQueryOptions(name: string) {
  return queryOptions({
    queryKey: gitRemoteKeys.detail(name),
    queryFn: () =>
      request<GitRemoteResource>(path(name), undefined, 'load git remote'),
  })
}

export function useGitRemote(name: string, enabled: boolean) {
  return useQuery({ ...gitRemoteQueryOptions(name), enabled })
}

export function useIssueGitRemote(name: string) {
  const queryClient = useQueryClient()
  return useMutation<GitRemoteResource, ApiError>({
    mutationFn: () =>
      request<GitRemoteResource>(path(name), { method: 'POST' }, 'issue token'),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: gitRemoteKeys.detail(name) }),
  })
}

export function useSetGitRemoteBranch(name: string) {
  const queryClient = useQueryClient()
  return useMutation<GitRemoteResource, ApiError, string>({
    mutationFn: (branch) =>
      request<GitRemoteResource>(
        path(name),
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ branch }),
        },
        'save branch',
      ),
    onSuccess: (remote) =>
      queryClient.setQueryData(gitRemoteKeys.detail(name), remote),
  })
}

export function useRevokeGitRemote(name: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError>({
    mutationFn: () =>
      request<void>(path(name), { method: 'DELETE' }, 'revoke remote'),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: gitRemoteKeys.detail(name) }),
  })
}
