// Query-key factory and fetchers for the deploy sub-resource of an app:
// GET/POST /api/v1/apps/{name}/deploys (internal/api/deploys.go). Kept
// in its own module rather than folded into queries/apps.ts because it
// is genuinely a different resource with a different shape (a condition
// list, not an AppDetail), even though it nests under the same app name.
//
// Important framing carried over from internal/api/deploys.go's own doc
// comment: GET here returns the app's *current* reconcile conditions,
// not a deploy history log. internal/store.UpsertConditions only ever
// persists the latest condition per (controller, condition type) pair.
// Nothing in this module should be named or shaped to imply an
// attempt-by-attempt log exists yet.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type { AppDetail } from '../types/appDetail'
import type { ReconcileCondition } from '../types/deploy'
import { appKeys } from './apps'
import { deployAttemptKeys } from './deployAttempts'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const deployKeys = {
  status: (appName: string) => [...appKeys.detail(appName), 'deploys'] as const,
}

export async function fetchDeployStatus(
  appName: string,
): Promise<ReconcileCondition[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/deploys`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch deploy status failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as ReconcileCondition[] | null
  return body ?? []
}

export function deployStatusQueryOptions(appName: string) {
  return queryOptions({
    queryKey: deployKeys.status(appName),
    queryFn: () => fetchDeployStatus(appName),
  })
}

export function useDeployStatus(appName: string) {
  return useSuspenseQuery(deployStatusQueryOptions(appName))
}

// POST /api/v1/apps/{name}/deploys (internal/api/apps.go's
// handleTriggerDeploy). Per that handler's own doc comment, this only
// points desired_services.image at a new tag; nothing here builds an
// image, streams progress, or watches the resulting reconcile complete.
// There is no backend SSE endpoint yet for a live build/deploy log, so
// this deliberately stays a fire-and-forget trigger, not a connection
// this hook keeps open.
//
// confirm must be true to deploy into an app tagged with a protected
// environment (internal/api's environmentNeedsConfirmation); callers
// derive it from ProtectedEnvironmentNotice's own acknowledgment
// checkbox, gated on useProtectedEnvironment (queries/environments.ts).
export interface TriggerDeployInput {
  image: string
  confirm?: boolean
}

export async function triggerDeploy(
  appName: string,
  input: TriggerDeployInput,
): Promise<AppDetail> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploys`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        image: input.image,
        confirm: input.confirm ?? false,
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `trigger deploy failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

// On success, updates both the app detail cache (the response is the
// updated AppDetail with its new image) and invalidates the deploy
// status query, since triggering a deploy changes what the next
// reconcile reports. Also invalidates the real deploy-attempts history
// list (queries/deployAttempts.ts): internal/api/deploys.go's
// handleTriggerDeploy now saves a real deploy_attempts row on every
// call (see that handler's own doc comment on why the plain image-tag
// path gets one too), so a caller watching DeployAttemptsList (the
// Deploys section, or DeployAttemptsList.tsx's own rollback button
// reusing this exact mutation) needs its history to refresh the moment
// a trigger succeeds, not on the next unrelated navigation.
export function useTriggerDeploy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: TriggerDeployInput) => triggerDeploy(appName, input),
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(appName), updated)
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(appName),
      })
    },
  })
}
