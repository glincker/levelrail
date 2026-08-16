// Query-key-free fetcher/mutation for POST /api/v1/apps/{name}/builds
// (internal/api/builds.go's handleTriggerBuild): the "build an image
// from a git source, then deploy it" counterpart to queries/deploys.ts's
// triggerDeploy ("deploy an already-built image tag"). Kept in its own
// module for the same reason queries/deploys.ts is: a distinct request
// shape and a distinct backend route, even though both ultimately update
// the same app resource.
//
// Important framing carried over from handleTriggerBuild's own doc
// comment: this call is synchronous and blocking on the server side,
// the same as internal/webhook.Handler.ServeHTTP's own call into
// internal/deploy.Pipeline. There is still no backend SSE endpoint for
// live build progress (web/src/hooks/useLogStream.ts's own doc
// comment: its SSE contract is "assumed," not backed by any real
// internal/api route), so this mutation's pending state is the only
// "building..." signal a caller gets, the same shape triggerDeploy
// already has for its own (much faster) operation, just held open for
// longer.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { AppDetail } from '../types/appDetail'
import type { StaticSite } from './staticSites'
import { appKeys } from './apps'
import { deployKeys } from './deploys'
import { deployAttemptKeys } from './deployAttempts'
import { staticSitesKeys } from './staticSites'
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

// TriggerBuildResult mirrors internal/api/builds.go's own conditional
// triggerBuildResponse exactly (field name static_site kept snake_case,
// matching this module's own wire-shape-as-is convention, see
// types/appDetail.ts's doc comment): a container-backed build
// (dockerfile, railpack) sets image/app, a build.type: static build
// sets static_site instead and leaves both of those unset, since a
// static deploy never touches store.DesiredService and its build has
// no image (see that Go type's own doc comment for why). Callers must
// check which one is present rather than assuming app is always there,
// the same "static is a genuinely different shape" awareness the
// backend response itself now carries.
export interface TriggerBuildResult {
  image?: string
  app?: AppDetail
  static_site?: StaticSite
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

// On success, updates both the app detail cache (the response carries
// the updated AppDetail with its new image, same as useTriggerDeploy)
// and invalidates the deploy status query, since a successful build
// changes what the next reconcile reports. Also invalidates the deploy-
// attempts history list, the same reasoning useTriggerDeploy's own doc
// comment gives: internal/api/builds.go's handleTriggerBuild now saves a
// real deploy_attempts row (running, then succeeded/failed) for every
// build, and a build is exactly the slower, more likely to be watched
// path where seeing the new row (and its live log link) appear promptly
// matters most.
//
// A build.type: static result carries no app (see TriggerBuildResult's
// own doc comment), so the app detail cache is left untouched rather
// than overwritten with undefined; the static-sites list query is
// invalidated instead, since that's the resource a static build
// actually just created or updated.
export function useTriggerBuild(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: TriggerBuildInput) => triggerBuild(appName, input),
    onSuccess: (result) => {
      if (result.app) {
        queryClient.setQueryData(appKeys.detail(appName), result.app)
      }
      if (result.static_site) {
        void queryClient.invalidateQueries({ queryKey: staticSitesKeys.all })
      }
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(appName),
      })
    },
  })
}
