// GET/PUT /api/v1/apps/{name}/github-actions (internal/api/app_github_actions.go):
// what the GitHub Actions deploy action reports back to the commit.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface GitHubActionsSetting {
  environment: string
  report_deployments: boolean
  report_checks: boolean
}

export const githubActionsKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'github-actions'] as const,
}

function url(appName: string) {
  return `/api/v1/apps/${encodeURIComponent(appName)}/github-actions`
}

async function fetchSetting(appName: string): Promise<GitHubActionsSetting> {
  const res = await fetch(url(appName))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch github actions setting failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitHubActionsSetting
}

export function useGitHubActionsSetting(appName: string) {
  return useSuspenseQuery(
    queryOptions({
      queryKey: githubActionsKeys.detail(appName),
      queryFn: () => fetchSetting(appName),
    }),
  )
}

async function putSetting(
  appName: string,
  input: GitHubActionsSetting,
): Promise<GitHubActionsSetting> {
  const res = await fetch(url(appName), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `save github actions setting failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitHubActionsSetting
}

export function useSetGitHubActionsSetting(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<GitHubActionsSetting, ApiError, GitHubActionsSetting>({
    mutationFn: (input) => putSetting(appName, input),
    onSuccess: (data) => {
      queryClient.setQueryData(githubActionsKeys.detail(appName), data)
    },
  })
}
