// Query-key factory and fetchers for the GitLab App connection resource
// (internal/api/gitlab_app.go, gitlab_app_oauth.go, gitlab_app_projects.go),
// following the same shared-queryOptions pattern queries/githubApp.ts
// already established.
//
// GET /api/v1/gitlab-app/connect is not a fetcher here for the same
// reason GitHub's register/start isn't in queries/githubApp.ts: it's a
// real, full-page browser navigation (window.location.href), GitLab's
// OAuth2 authorize endpoint requires that. See GitLabAppConnectionCard.tsx
// for where that navigation happens.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  GitLabAppConnectRequest,
  GitLabAppProject,
  GitLabAppStatus,
  GitLabAppUseProjectAsSourceRequest,
} from '../types/gitlabApp'
import type { GitSourceResource } from '../types/gitSource'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { gitSourceKeys } from './gitSources'

export const gitlabAppKeys = {
  all: ['gitlab-app'] as const,
  status: () => [...gitlabAppKeys.all, 'status'] as const,
  projects: () => [...gitlabAppKeys.all, 'projects'] as const,
}

export async function fetchGitLabAppStatus(): Promise<GitLabAppStatus> {
  const res = await fetch('/api/v1/gitlab-app')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch gitlab app status failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitLabAppStatus
}

export function gitlabAppStatusQueryOptions() {
  return queryOptions({
    queryKey: gitlabAppKeys.status(),
    queryFn: fetchGitLabAppStatus,
  })
}

export function useGitLabAppStatus() {
  return useSuspenseQuery(gitlabAppStatusQueryOptions())
}

// PUT /api/v1/gitlab-app (handleConnectGitLabApp): saves the OAuth
// Application's own instance/client_id/client_secret. Does not itself
// obtain an access token; the "Connect" button navigates to
// GET /api/v1/gitlab-app/connect separately for that.
export async function connectGitLabApp(
  req: GitLabAppConnectRequest,
): Promise<GitLabAppStatus> {
  const res = await fetch('/api/v1/gitlab-app', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `connect gitlab app failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitLabAppStatus
}

export function useConnectGitLabApp() {
  const queryClient = useQueryClient()
  return useMutation<GitLabAppStatus, ApiError, GitLabAppConnectRequest>({
    mutationFn: connectGitLabApp,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: gitlabAppKeys.all })
    },
  })
}

// DELETE /api/v1/gitlab-app (handleDisconnectGitLabApp). 204 on success.
export async function disconnectGitLabApp(): Promise<void> {
  const res = await fetch('/api/v1/gitlab-app', { method: 'DELETE' })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `disconnect gitlab app failed: ${res.status}`),
  )
}

export function useDisconnectGitLabApp() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: disconnectGitLabApp,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: gitlabAppKeys.all })
    },
  })
}

// GET /api/v1/gitlab-app/projects (handleListGitLabAppProjects). A plain
// useQuery, not useSuspenseQuery: only fetched once the connection is
// authorized, the same "enabled gates it" shape useGitHubAppRepos uses.
export async function fetchGitLabAppProjects(): Promise<GitLabAppProject[]> {
  const res = await fetch('/api/v1/gitlab-app/projects')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch gitlab app projects failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitLabAppProject[]
}

export function useGitLabAppProjects(enabled: boolean) {
  return useQuery({
    queryKey: gitlabAppKeys.projects(),
    queryFn: fetchGitLabAppProjects,
    enabled,
  })
}

// POST /api/v1/gitlab-app/projects/{id}/use-as-source
// (handleUseGitLabProjectAsSource): connects the project as appName's
// git source through the same store.GitSource row PUT
// .../git-source itself creates, and registers a GitLab project
// webhook. Response is the identical GitSourceResource shape PUT
// .../git-source returns, so on success this seeds that exact query's
// cache too, the same way useSetGitSource already does.
export async function connectGitLabProjectAsSource(
  projectID: number,
  req: GitLabAppUseProjectAsSourceRequest,
): Promise<GitSourceResource> {
  const res = await fetch(`/api/v1/gitlab-app/projects/${projectID}/use-as-source`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `use gitlab project as source failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourceResource
}

export function useConnectGitLabProjectAsSource() {
  const queryClient = useQueryClient()
  return useMutation<
    GitSourceResource,
    ApiError,
    { projectID: number; req: GitLabAppUseProjectAsSourceRequest }
  >({
    mutationFn: ({ projectID, req }) => connectGitLabProjectAsSource(projectID, req),
    onSuccess: (resource, variables) => {
      queryClient.setQueryData(gitSourceKeys.detail(variables.req.app_name), resource)
    },
  })
}
