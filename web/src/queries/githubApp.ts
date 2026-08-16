// Query-key factory and fetchers for the GitHub App connection resource
// (internal/api/github_app.go, github_app_register.go,
// github_app_repos.go), following the same shared-queryOptions pattern
// queries/backupTargets.ts and queries/domains.ts already established.
//
// This only covers what internal/api actually exposes: connection
// status, disconnect, and repo/branch listing through an already-
// installed App. The manifest registration flow itself
// (GET /api/v1/github-app/register/start) is not a fetcher here: it is
// a real, full-page browser navigation (window.location.href, not
// fetch), the same way navigating to GitHub's own confirmation page and
// back has to be. See GitHubAppConnectionCard.tsx for where that
// navigation actually happens.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  GitHubAppBranch,
  GitHubAppManualConnectRequest,
  GitHubAppRepo,
  GitHubAppStatus,
} from '../types/githubApp'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const githubAppKeys = {
  all: ['github-app'] as const,
  status: () => [...githubAppKeys.all, 'status'] as const,
  repos: () => [...githubAppKeys.all, 'repos'] as const,
  branches: (owner: string, repo: string) =>
    [...githubAppKeys.all, 'branches', owner, repo] as const,
}

// GET /api/v1/github-app (handleGetGitHubAppStatus). AbilityRoot on the
// backend, but every session is implicitly root (requireAbility's own
// doc comment), so the interactive settings page this feeds is not
// itself gated any differently than any other AbilityRoot-only page in
// this frontend.
export async function fetchGitHubAppStatus(): Promise<GitHubAppStatus> {
  const res = await fetch('/api/v1/github-app')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch github app status failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitHubAppStatus
}

export function githubAppStatusQueryOptions() {
  return queryOptions({
    queryKey: githubAppKeys.status(),
    queryFn: fetchGitHubAppStatus,
  })
}

export function useGitHubAppStatus() {
  return useSuspenseQuery(githubAppStatusQueryOptions())
}

// DELETE /api/v1/github-app (handleDisconnectGitHubApp). 204 on
// success, no body to parse.
export async function disconnectGitHubApp(): Promise<void> {
  const res = await fetch('/api/v1/github-app', { method: 'DELETE' })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `disconnect github app failed: ${res.status}`),
  )
}

export function useDisconnectGitHubApp() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: disconnectGitHubApp,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: githubAppKeys.all })
    },
  })
}

// PUT /api/v1/github-app/manual (handleConnectGitHubAppManually): the
// alternative to the automated manifest flow (register/start above),
// for an operator who created the App by hand on github.com, or whose
// primary domain isn't configured/reachable yet.
export async function connectGitHubAppManually(
  req: GitHubAppManualConnectRequest,
): Promise<GitHubAppStatus> {
  const res = await fetch('/api/v1/github-app/manual', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `connect github app manually failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitHubAppStatus
}

export function useConnectGitHubAppManually() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, GitHubAppManualConnectRequest>({
    mutationFn: async (req) => {
      await connectGitHubAppManually(req)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: githubAppKeys.all })
    },
  })
}

// GET /api/v1/github-app/repos (handleListGitHubAppRepos). A plain
// useQuery, not useSuspenseQuery: this is only fetched once a repo
// picker is actually shown (the App must be connected and installed
// first), so `enabled` gates it rather than every caller needing a
// Suspense boundary for a query that often legitimately never runs.
// 409 (not connected / not installed) and 501 (no master key) are both
// real, expected states here, not just error noise: callers read
// error.status to render the right empty state instead of a generic
// failure message.
export async function fetchGitHubAppRepos(): Promise<GitHubAppRepo[]> {
  const res = await fetch('/api/v1/github-app/repos')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch github app repos failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitHubAppRepo[]
}

export function useGitHubAppRepos(enabled: boolean) {
  return useQuery({
    queryKey: githubAppKeys.repos(),
    queryFn: fetchGitHubAppRepos,
    enabled,
  })
}

// GET /api/v1/github-app/repos/{owner}/{repo}/branches
// (handleListGitHubAppBranches).
export async function fetchGitHubAppBranches(
  owner: string,
  repo: string,
): Promise<GitHubAppBranch[]> {
  const res = await fetch(
    `/api/v1/github-app/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/branches`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch github app branches failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitHubAppBranch[]
}

export function useGitHubAppBranches(
  owner: string,
  repo: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: githubAppKeys.branches(owner, repo),
    queryFn: () => fetchGitHubAppBranches(owner, repo),
    enabled: enabled && owner !== '' && repo !== '',
  })
}
