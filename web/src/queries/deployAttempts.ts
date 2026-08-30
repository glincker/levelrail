// Query-key factory and fetcher for GET /api/v1/apps/{name}/deploy-attempts
// (internal/api/deploy_attempts.go's handleListDeployAttempts): real,
// row-per-attempt deploy history, newest first. Kept in its own module,
// separate from queries/deploys.ts, for the same reason that file's own
// doc comment gives for GET /apps/{name}/deploys: the two backend
// endpoints are genuinely different resources (current reconcile status
// vs. a history log) even though both nest under the same app name, and
// queries/deploys.ts's own doc comment is explicit that nothing there
// should be shaped to imply an attempt-by-attempt log, this module is
// that log.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import type { DeployAttempt } from '../types/deployAttempt'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const deployAttemptKeys = {
  list: (appName: string) =>
    [...appKeys.detail(appName), 'deploy-attempts'] as const,
}

export async function fetchDeployAttempts(
  appName: string,
): Promise<DeployAttempt[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploy-attempts`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch deploy attempts failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as DeployAttempt[] | null
  return body ?? []
}

// Polls while the app's latest attempt is still 'running' (newest-first,
// so attempts[0]), so the deploy-in-progress banner and the stage
// timeline pick up a build finishing or a rollback landing without the
// operator needing to navigate away and back. Off (false) once nothing
// is running: matches queries/activity.ts's own refetchInterval
// precedent, just conditional instead of constant.
const RUNNING_ATTEMPT_POLL_INTERVAL_MS = 3_000

export function deployAttemptsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: deployAttemptKeys.list(appName),
    queryFn: () => fetchDeployAttempts(appName),
    refetchInterval: (query) =>
      query.state.data?.[0]?.status === 'running'
        ? RUNNING_ATTEMPT_POLL_INTERVAL_MS
        : false,
  })
}

export function useDeployAttempts(appName: string) {
  return useSuspenseQuery(deployAttemptsQueryOptions(appName))
}
