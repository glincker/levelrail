// PUT /api/v1/apps/{name}/git-source/deploy-settings
// (internal/api/git_deploy_settings.go): push path filters and forge status
// reporting for an app's connected git source.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type {
  GitDeploySettings,
  GitSourceResource,
  SetGitDeploySettingsRequest,
} from '../types/gitSource'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { gitSourceKeys } from './gitSources'

export async function setGitDeploySettings(
  name: string,
  req: SetGitDeploySettingsRequest,
): Promise<GitDeploySettings> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/git-source/deploy-settings`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `save deploy settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitDeploySettings
}

export function useSetGitDeploySettings(name: string) {
  const queryClient = useQueryClient()
  return useMutation<GitDeploySettings, ApiError, SetGitDeploySettingsRequest>({
    mutationFn: (req) => setGitDeploySettings(name, req),
    onSuccess: (settings) => {
      queryClient.setQueryData<GitSourceResource>(
        gitSourceKeys.detail(name),
        (prev) => (prev ? { ...prev, ...settings } : prev),
      )
    },
  })
}
