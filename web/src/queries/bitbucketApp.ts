// Query-key factory and fetchers for the Bitbucket App connection
// resource (internal/api/bitbucket_app.go, bitbucket_app_oauth.go,
// bitbucket_app_repos.go), following the same shared-queryOptions
// pattern queries/gitlabApp.ts already established.
//
// GET /api/v1/bitbucket-app/connect is not a fetcher here for the same
// reason GitLab's own connect endpoint isn't in queries/gitlabApp.ts:
// it's a real, full-page browser navigation (window.location.href),
// Bitbucket's OAuth2 authorize endpoint requires that. See
// BitbucketAppConnectionCard.tsx for where that navigation happens.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  BitbucketAppBranch,
  BitbucketAppConnectRequest,
  BitbucketAppRepo,
  BitbucketAppStatus,
  BitbucketAppUseRepoAsSourceRequest,
} from '../types/bitbucketApp'
import type { GitSourceResource } from '../types/gitSource'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { gitSourceKeys } from './gitSources'

export const bitbucketAppKeys = {
  all: ['bitbucket-app'] as const,
  status: () => [...bitbucketAppKeys.all, 'status'] as const,
  repos: () => [...bitbucketAppKeys.all, 'repos'] as const,
  branches: (workspace: string, repoSlug: string) =>
    [...bitbucketAppKeys.all, 'branches', workspace, repoSlug] as const,
}

export async function fetchBitbucketAppStatus(): Promise<BitbucketAppStatus> {
  const res = await fetch('/api/v1/bitbucket-app')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch bitbucket app status failed: ${res.status}`),
    )
  }
  return (await res.json()) as BitbucketAppStatus
}

export function bitbucketAppStatusQueryOptions() {
  return queryOptions({
    queryKey: bitbucketAppKeys.status(),
    queryFn: fetchBitbucketAppStatus,
  })
}

export function useBitbucketAppStatus() {
  return useSuspenseQuery(bitbucketAppStatusQueryOptions())
}

// PUT /api/v1/bitbucket-app (handleConnectBitbucketApp): saves the
// OAuth consumer's own key/secret. Does not itself obtain an access
// token; the "Connect" button navigates to
// GET /api/v1/bitbucket-app/connect separately for that.
export async function connectBitbucketApp(
  req: BitbucketAppConnectRequest,
): Promise<BitbucketAppStatus> {
  const res = await fetch('/api/v1/bitbucket-app', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `connect bitbucket app failed: ${res.status}`),
    )
  }
  return (await res.json()) as BitbucketAppStatus
}

export function useConnectBitbucketApp() {
  const queryClient = useQueryClient()
  return useMutation<BitbucketAppStatus, ApiError, BitbucketAppConnectRequest>({
    mutationFn: connectBitbucketApp,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: bitbucketAppKeys.all })
    },
  })
}

// DELETE /api/v1/bitbucket-app (handleDisconnectBitbucketApp). 204 on
// success.
export async function disconnectBitbucketApp(): Promise<void> {
  const res = await fetch('/api/v1/bitbucket-app', { method: 'DELETE' })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `disconnect bitbucket app failed: ${res.status}`),
  )
}

export function useDisconnectBitbucketApp() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: disconnectBitbucketApp,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: bitbucketAppKeys.all })
    },
  })
}

// GET /api/v1/bitbucket-app/repos (handleListBitbucketAppRepos). A
// plain useQuery, not useSuspenseQuery: only fetched once the
// connection is authorized, the same "enabled gates it" shape
// useGitLabAppProjects/useGitHubAppRepos use.
export async function fetchBitbucketAppRepos(): Promise<BitbucketAppRepo[]> {
  const res = await fetch('/api/v1/bitbucket-app/repos')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch bitbucket app repos failed: ${res.status}`),
    )
  }
  return (await res.json()) as BitbucketAppRepo[]
}

export function useBitbucketAppRepos(enabled: boolean) {
  return useQuery({
    queryKey: bitbucketAppKeys.repos(),
    queryFn: fetchBitbucketAppRepos,
    enabled,
  })
}

// GET /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/branches
// (handleListBitbucketAppBranches).
export async function fetchBitbucketAppBranches(
  workspace: string,
  repoSlug: string,
): Promise<BitbucketAppBranch[]> {
  const res = await fetch(
    `/api/v1/bitbucket-app/repos/${encodeURIComponent(workspace)}/${encodeURIComponent(repoSlug)}/branches`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch bitbucket app branches failed: ${res.status}`),
    )
  }
  return (await res.json()) as BitbucketAppBranch[]
}

export function useBitbucketAppBranches(
  workspace: string,
  repoSlug: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: bitbucketAppKeys.branches(workspace, repoSlug),
    queryFn: () => fetchBitbucketAppBranches(workspace, repoSlug),
    enabled: enabled && workspace !== '' && repoSlug !== '',
  })
}

// POST /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/use-as-source
// (handleUseBitbucketRepoAsSource): connects the repo as appName's git
// source through the same store.GitSource row PUT .../git-source itself
// creates, and registers a Bitbucket repo webhook. Response is the
// identical GitSourceResource shape PUT .../git-source returns, the
// same cache-seeding shape useConnectGitLabProjectAsSource already has.
export async function connectBitbucketRepoAsSource(
  workspace: string,
  repoSlug: string,
  req: BitbucketAppUseRepoAsSourceRequest,
): Promise<GitSourceResource> {
  const res = await fetch(
    `/api/v1/bitbucket-app/repos/${encodeURIComponent(workspace)}/${encodeURIComponent(repoSlug)}/use-as-source`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `use bitbucket repo as source failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourceResource
}

export function useConnectBitbucketRepoAsSource() {
  const queryClient = useQueryClient()
  return useMutation<
    GitSourceResource,
    ApiError,
    { workspace: string; repoSlug: string; req: BitbucketAppUseRepoAsSourceRequest }
  >({
    mutationFn: ({ workspace, repoSlug, req }) =>
      connectBitbucketRepoAsSource(workspace, repoSlug, req),
    onSuccess: (resource, variables) => {
      queryClient.setQueryData(gitSourceKeys.detail(variables.req.app_name), resource)
    },
  })
}
