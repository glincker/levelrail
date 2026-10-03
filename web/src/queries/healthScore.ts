// Query-key factory and fetcher for GET
// /api/v1/apps/{name}/health-score (internal/api/app_health_score.go).
// Kept in its own module for the same reason queries/alerts.ts is: a
// genuinely different resource shape than AppDetail, nested under the
// same app name.
//
// No suspense/loader-primed query here, same reasoning queries/alerts.ts
// gives for AlertRulesPanel: AppHealthScorePanel is a plain section on
// the app overview page, not something the route loader needs warm
// before first paint.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { AppHealthScore } from '../types/healthScore'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const healthScoreKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'health-score'] as const,
}

export async function fetchAppHealthScore(
  appName: string,
): Promise<AppHealthScore> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/health-score`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch health score failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppHealthScore
}

export function appHealthScoreQueryOptions(appName: string) {
  return queryOptions({
    queryKey: healthScoreKeys.detail(appName),
    queryFn: () => fetchAppHealthScore(appName),
    // Deploy/crashloop/cert signals change on their own timelines, not
    // only when this tab is open; a short poll keeps the panel from
    // going stale while an operator is looking at it, the same
    // "quiet idle cost, fresh while watched" shape FleetUtilizationSummary
    // already uses for its own 30s poll (docs/observability.md).
    refetchInterval: 30_000,
  })
}

export function useAppHealthScore(appName: string) {
  return useQuery(appHealthScoreQueryOptions(appName))
}
