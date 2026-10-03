// Query-key factory and fetcher for GET
// /api/v1/apps/{name}/cost-estimate (internal/api's
// handleAppCostEstimate). Fetched lazily (plain useQuery with
// `enabled`), the same shape queries/resourceRecommendation.ts already
// uses: this should only fire when the app detail page's resources tab
// actually renders the cost card.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { CostEstimate } from '../types/costEstimate'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const costEstimateKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'cost-estimate'] as const,
}

export async function fetchCostEstimate(
  appName: string,
): Promise<CostEstimate> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/cost-estimate`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch cost estimate failed: ${res.status}`),
    )
  }
  return (await res.json()) as CostEstimate
}

export function costEstimateQueryOptions(appName: string) {
  return queryOptions({
    queryKey: costEstimateKeys.detail(appName),
    queryFn: () => fetchCostEstimate(appName),
  })
}

export function useCostEstimate(appName: string, enabled = true) {
  return useQuery({ ...costEstimateQueryOptions(appName), enabled })
}
