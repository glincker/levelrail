// Query-key-free fetcher/mutation for POST /api/v1/apps/{name}/builds
// (internal/api/builds.go's handleTriggerBuild): the "build an image
// from a git source, then deploy it" counterpart to queries/deploys.ts's
// triggerDeploy ("deploy an already-built image tag"). Kept in its own
// module for the same reason queries/deploys.ts is: a distinct request
// shape and a distinct backend route, even though both ultimately update
// the same app resource.
//
// The build runs asynchronously on the server: handleTriggerBuild
// returns as soon as the deploy attempt is recorded, well before the
// build itself has done any work, so this call's response carries only
// the attempt id, never a built image or an updated app. Poll
// deployAttemptKeys.list(appName) or tail GET .../deploys/{id}/logs to
// observe progress and the eventual outcome.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { deployKeys } from './deploys'
import { deployAttemptKeys } from './deployAttempts'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface TriggerBuildInput {
  repoUrl: string
  ref: string
  /** Defaults to the app's own name on the backend when omitted. */
  imageRepo?: string
  /** Defaults to "dockerfile" on the backend when omitted. */
  buildType?: string
  buildPath?: string
}

// TriggerBuildResult mirrors internal/api/builds.go's own
// triggerBuildResponse: id is the deploy_attempts row's own id
// (deploy_attempts.go's deployAttemptResource uses the same "id" tag for
// the identical field), empty when attempt recording itself failed
// (see beginBuildDeployAttempt's own doc comment on why that never
// blocks the build).
export interface TriggerBuildResult {
  id?: string
}

export async function triggerBuild(
  appName: string,
  input: TriggerBuildInput,
): Promise<TriggerBuildResult> {
  const build: Record<string, string> = {}
  if (input.buildType) build.type = input.buildType
  if (input.buildPath) build.path = input.buildPath

  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/builds`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        repo_url: input.repoUrl,
        ref: input.ref,
        ...(input.imageRepo ? { image_repo: input.imageRepo } : {}),
        ...(Object.keys(build).length > 0 ? { build } : {}),
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `trigger build failed: ${res.status}`),
    )
  }
  return (await res.json()) as TriggerBuildResult
}

// On success, invalidates the deploy status query and the deploy-
// attempts history list: the build itself hasn't finished yet (see this
// module's own doc comment), so there is no updated app or static site
// to write into the cache, only a new "running" deploy_attempts row
// (this response's own id) for the attempts list and log viewer to pick
// up. Callers should treat onSuccess as "the build was triggered," not
// "the build finished."
export function useTriggerBuild(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: TriggerBuildInput) => triggerBuild(appName, input),
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
