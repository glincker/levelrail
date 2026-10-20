// Query-key factory and fetchers for the Gitea App connection resource
// (internal/api/gitea_app.go, gitea_app_oauth.go, gitea_app_repos.go),
// following the same shared-queryOptions pattern queries/gitlabApp.ts
// already established (self-hosted instance_url), combined with
// queries/bitbucketApp.ts's own two-segment owner/repo path shape.
//
// GET /api/v1/gitea-app/connect is not a fetcher here for the same
// reason GitLab's own connect endpoint isn't in queries/gitlabApp.ts:
// it's a real, full-page browser navigation (window.location.href),
// Gitea's OAuth2 authorize endpoint requires that. See
// GiteaAppConnectionCard.tsx for where that navigation happens.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  GiteaAppBranch,
  GiteaAppConnectRequest,
  GiteaAppRepo,
  GiteaAppStatus,
  GiteaAppUseRepoAsSourceRequest,
} from '../types/giteaApp'
import type { GitSourceResource } from '../types/gitSource'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { gitSourceKeys } from './gitSources'

export const giteaAppKeys = {
  all: ['gitea-app'] as const,
  status: () => [...giteaAppKeys.all, 'status'] as const,
  repos: () => [...giteaAppKeys.all, 'repos'] as const,
  branches: (owner: string, repo: string) =>
    [...giteaAppKeys.all, 'branches', owner, repo] as const,
}

export async function fetchGiteaAppStatus(): Promise<GiteaAppStatus> {
  const res = await fetch('/api/v1/gitea-app')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch gitea app status failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GiteaAppStatus
}

export function giteaAppStatusQueryOptions() {
  return queryOptions({
    queryKey: giteaAppKeys.status(),
    queryFn: fetchGiteaAppStatus,
  })
}

export function useGiteaAppStatus() {
  return useSuspenseQuery(giteaAppStatusQueryOptions())
}

// PUT /api/v1/gitea-app (handleConnectGiteaApp): saves the OAuth
// Application's own instance/client_id/client_secret. Does not itself
// obtain an access token; the "Connect" button navigates to
// GET /api/v1/gitea-app/connect separately for that.
export async function connectGiteaApp(
  req: GiteaAppConnectRequest,
): Promise<GiteaAppStatus> {
  const res = await fetch('/api/v1/gitea-app', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `connect gitea app failed: ${res.status}`),
    )
  }
  return (await res.json()) as GiteaAppStatus
}

export function useConnectGiteaApp() {
  const queryClient = useQueryClient()
  return useMutation<GiteaAppStatus, ApiError, GiteaAppConnectRequest>({
    mutationFn: connectGiteaApp,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: giteaAppKeys.all })
    },
  })
}

// DELETE /api/v1/gitea-app (handleDisconnectGiteaApp). 204 on success.
export async function disconnectGiteaApp(): Promise<void> {
  const res = await fetch('/api/v1/gitea-app', { method: 'DELETE' })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `disconnect gitea app failed: ${res.status}`),
  )
}

export function useDisconnectGiteaApp() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: disconnectGiteaApp,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: giteaAppKeys.all })
    },
  })
}

// GET /api/v1/gitea-app/repos (handleListGiteaAppRepos). A plain
// useQuery, not useSuspenseQuery: only fetched once the connection is
// authorized, the same "enabled gates it" shape useGitLabAppProjects/
// useBitbucketAppRepos use.
export async function fetchGiteaAppRepos(): Promise<GiteaAppRepo[]> {
  const res = await fetch('/api/v1/gitea-app/repos')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch gitea app repos failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GiteaAppRepo[]
}

export function useGiteaAppRepos(enabled: boolean) {
  return useQuery({
    queryKey: giteaAppKeys.repos(),
    queryFn: fetchGiteaAppRepos,
    enabled,
  })
}

// GET /api/v1/gitea-app/repos/{owner}/{repo}/branches
// (handleListGiteaAppBranches).
export async function fetchGiteaAppBranches(
  owner: string,
  repo: string,
): Promise<GiteaAppBranch[]> {
  const res = await fetch(
    `/api/v1/gitea-app/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/branches`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch gitea app branches failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GiteaAppBranch[]
}

export function useGiteaAppBranches(
  owner: string,
  repo: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: giteaAppKeys.branches(owner, repo),
    queryFn: () => fetchGiteaAppBranches(owner, repo),
    enabled: enabled && owner !== '' && repo !== '',
  })
}

// POST /api/v1/gitea-app/repos/{owner}/{repo}/use-as-source
// (handleUseGiteaRepoAsSource): connects the repo as appName's git
// source through the same store.GitSource row PUT .../git-source itself
// creates, and registers a Gitea repo webhook. Response is the
// identical GitSourceResource shape PUT .../git-source returns, the
// same cache-seeding shape useConnectBitbucketRepoAsSource already has.
export async function connectGiteaRepoAsSource(
  owner: string,
  repo: string,
  req: GiteaAppUseRepoAsSourceRequest,
): Promise<GitSourceResource> {
  const res = await fetch(
    `/api/v1/gitea-app/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/use-as-source`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `use gitea repo as source failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitSourceResource
}

export function useConnectGiteaRepoAsSource() {
  const queryClient = useQueryClient()
  return useMutation<
    GitSourceResource,
    ApiError,
    { owner: string; repo: string; req: GiteaAppUseRepoAsSourceRequest }
  >({
    mutationFn: ({ owner, repo, req }) =>
      connectGiteaRepoAsSource(owner, repo, req),
    onSuccess: (resource, variables) => {
      queryClient.setQueryData(
        gitSourceKeys.detail(variables.req.app_name),
        resource,
      )
    },
  })
}
