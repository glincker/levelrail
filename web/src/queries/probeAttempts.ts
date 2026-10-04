// Query-key factory and fetcher for GET
// /api/v1/apps/{name}/deploys/{deployId}/probes
// (internal/api/deploy_probes.go's handleListProbeAttempts): a bounded,
// already-terminal-by-replay list, so this is plain polling JSON through
// Query's own cache, not an SSE hook like queries/deploySteps.ts's URL
// builder (there is no live tail to subscribe to; see the backend
// handler's own doc comment for why).

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { ProbeAttempt } from '../types/probeAttempt'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const probeAttemptKeys = {
  list: (appName: string, deployId: string) =>
    [...appKeys.detail(appName), 'deploys', deployId, 'probes'] as const,
}

export async function fetchProbeAttempts(
  appName: string,
  deployId: string,
): Promise<ProbeAttempt[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploys/${encodeURIComponent(deployId)}/probes`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch probe attempts failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as ProbeAttempt[] | null
  return body ?? []
}

// Polls only while the deploy attempt is still running, matching
// queries/deployAttempts.ts's own conditional-refetch precedent: once an
// attempt is terminal, its probe attempts are final too.
const PROBE_ATTEMPT_POLL_INTERVAL_MS = 3_000

export function probeAttemptsQueryOptions(
  appName: string,
  deployId: string,
  isRunning: boolean,
) {
  return queryOptions({
    queryKey: probeAttemptKeys.list(appName, deployId),
    queryFn: () => fetchProbeAttempts(appName, deployId),
    refetchInterval: isRunning ? PROBE_ATTEMPT_POLL_INTERVAL_MS : false,
  })
}

export function useProbeAttempts(
  appName: string,
  deployId: string,
  isRunning: boolean,
) {
  return useQuery(probeAttemptsQueryOptions(appName, deployId, isRunning))
}
