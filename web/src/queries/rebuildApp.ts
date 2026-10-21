import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { ApiError } from '../lib/apiError'
import { triggerBuild } from './builds'
import type { TriggerBuildResult } from './builds'
import { deployAttemptKeys } from './deployAttempts'
import { deployKeys } from './deploys'
import { fetchGitSource } from './gitSources'

export interface RebuildAppInput {
  noCache: boolean
  rerunOf?: string
}

/** Rebuilds a git-source app from its branch head through POST /api/v1/apps/{name}/builds. */
export async function rebuildApp(
  appName: string,
  { noCache, rerunOf }: RebuildAppInput,
): Promise<TriggerBuildResult> {
  const gs = await fetchGitSource(appName)
  return triggerBuild(appName, {
    repoUrl: gs.repo_url,
    ref: gs.branch,
    buildType: gs.build_type,
    buildPath: gs.build_path,
    baseDirectory: gs.base_directory,
    noCache,
    rerunOf,
  })
}

export function useRebuildApp(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<TriggerBuildResult, ApiError, RebuildAppInput>({
    mutationFn: (input) => rebuildApp(appName, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(appName),
      })
    },
  })
}
