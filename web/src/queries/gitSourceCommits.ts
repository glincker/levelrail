// GET /api/v1/apps/{name}/git-source/commits (internal/api/git_source_commits.go):
// the newest commits of a branch with what is deployed against them, plus
// computed deploy suggestions. A 404 means no git source is connected.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type CommitDeployStatus =
  | 'deployed'
  | 'succeeded'
  | 'failed'
  | 'running'
  | 'queued'
  | 'held'
  | 'canceled'
  | 'superseded'

export interface GitCommit {
  sha: string
  message: string
  author: string
  authored_at: string
  url?: string
  deploy_status?: CommitDeployStatus
  deploy_id?: string
}

export interface GitDeployRef {
  deploy_id: string
  image: string
  commit_sha?: string
  finished_at: string
}

export type DeploySuggestionActionType =
  | 'open_source_settings'
  | 'open_build_settings'
  | 'enable_previews'
  | 'open_health_settings'
  | 'open_notifications'
  | 'open_deploy_logs'

export interface DeploySuggestion {
  id: string
  severity: 'warning' | 'info'
  reason: string
  params?: Record<string, string>
  action: { type: DeploySuggestionActionType; deploy_id?: string }
}

export interface GitSourceCommitsResponse {
  repo_url: string
  branch: string
  connected_branch: string
  provider: string
  head?: GitCommit
  commits: GitCommit[]
  branches: string[]
  deployed?: GitDeployRef
  ahead_by?: number
  up_to_date: boolean
  last_successful?: GitDeployRef
  previous_successful?: GitDeployRef
  build: { context_dir: string; dockerfile_path?: string; summary: string }
  build_type: string
  trigger_mode: string
  suggestions: DeploySuggestion[]
  degraded: boolean
  fetched_at: string
}

export const gitSourceCommitsKeys = {
  list: (appName: string, branch: string) =>
    ['git-source-commits', appName, branch] as const,
}

export async function fetchGitSourceCommits(
  appName: string,
  branch: string,
  refresh: boolean,
): Promise<GitSourceCommitsResponse> {
  const params = new URLSearchParams()
  if (branch) params.set('branch', branch)
  if (refresh) params.set('refresh', '1')
  const qs = params.toString()
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/git-source/commits${qs ? `?${qs}` : ''}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch commits failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourceCommitsResponse
}

const COMMITS_STALE_MS = 15_000

export function gitSourceCommitsQueryOptions(appName: string, branch: string) {
  return queryOptions({
    queryKey: gitSourceCommitsKeys.list(appName, branch),
    queryFn: () => fetchGitSourceCommits(appName, branch, false),
    staleTime: COMMITS_STALE_MS,
    retry: false,
  })
}

export function useGitSourceCommits(
  appName: string,
  branch: string,
  enabled: boolean,
) {
  return useQuery({
    ...gitSourceCommitsQueryOptions(appName, branch),
    enabled,
  })
}
