import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { GitSourceBuildType } from '../types/gitSource'

export interface GitSourcePlanPreserved {
  env_vars: number
  secret_env: number
  domains: string[]
  volumes: string[]
  replicas: number
  running_image?: string
}

export interface GitSourcePlanSuggestion {
  build_type: GitSourceBuildType
  build_path?: string
  base_directory?: string
  build_command?: string
  publish_dir?: string
  framework?: string
  reason?: string
}

export type GitSourceCurrentKind = 'git' | 'image' | 'upload' | 'none'

export interface GitSourcePlan {
  app: string
  current_source: GitSourceCurrentKind
  current_repo?: string
  static: boolean
  branch?: string
  preserved: GitSourcePlanPreserved
  suggestion?: GitSourcePlanSuggestion
  detect_error?:
    | 'not_found'
    | 'access_denied'
    | 'unreadable'
    | 'ssh_not_detectable'
    | 'unsupported_host'
  warnings: string[]
}

export const gitSourceConnectKeys = {
  plan: (name: string, repoUrl: string, branch: string) =>
    ['git-sources', 'plan', name, repoUrl, branch] as const,
}

export async function fetchGitSourcePlan(
  name: string,
  repoUrl: string,
  branch: string,
): Promise<GitSourcePlan> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/git-source/plan`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ repo_url: repoUrl, branch }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `plan git source failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourcePlan
}

// Plan persists nothing, so it reruns freely as the picked repo or branch changes.
export function gitSourcePlanOptions(
  name: string,
  repoUrl: string,
  branch: string,
) {
  return queryOptions({
    queryKey: gitSourceConnectKeys.plan(name, repoUrl, branch),
    queryFn: () => fetchGitSourcePlan(name, repoUrl, branch),
    retry: false,
  })
}

export function useGitSourcePlan(
  name: string,
  repoUrl: string,
  branch: string,
  enabled: boolean,
) {
  return useQuery({ ...gitSourcePlanOptions(name, repoUrl, branch), enabled })
}

export interface GitSourceDeployResult {
  app: string
  branch: string
  started: boolean
}

export async function deployGitSource(
  name: string,
  branch?: string,
): Promise<GitSourceDeployResult> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/git-source/deploy`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(branch ? { branch } : {}),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `start deploy failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourceDeployResult
}

export function useDeployGitSource(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (branch?: string) => deployGitSource(name, branch),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['deploys'] })
    },
  })
}
