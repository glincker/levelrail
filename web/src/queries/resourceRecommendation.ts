// Query-key factory and fetcher for GET
// /api/v1/apps/{name}/resource-recommendation (internal/api's
// handleAppResourceRecommendation). Fetched lazily (plain useQuery with
// `enabled`), the same shape queries/diagnosis.ts already uses: this
// should only fire when the resource-limits editor actually renders the
// suggestion card, not on every app page load.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { ResourceRecommendation } from '../types/resourceRecommendation'
import { appKeys } from './apps'
import { databaseKeys } from './databases'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const resourceRecommendationKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'resource-recommendation'] as const,
}

export async function fetchResourceRecommendation(
  appName: string,
): Promise<ResourceRecommendation> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/resource-recommendation`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch resource recommendation failed: ${res.status}`),
    )
  }
  return (await res.json()) as ResourceRecommendation
}

export function resourceRecommendationQueryOptions(appName: string) {
  return queryOptions({
    queryKey: resourceRecommendationKeys.detail(appName),
    queryFn: () => fetchResourceRecommendation(appName),
  })
}

export function useResourceRecommendation(appName: string, enabled = true) {
  return useQuery({ ...resourceRecommendationQueryOptions(appName), enabled })
}

// Database-kind counterpart to the app fetcher/hook above: same wire
// shape (internal/apiclient.ResourceRecommendationResource is shared by
// both endpoints), only the resource ID and URL differ.
export const databaseResourceRecommendationKeys = {
  detail: (databaseName: string) =>
    [...databaseKeys.detail(databaseName), 'resource-recommendation'] as const,
}

export async function fetchDatabaseResourceRecommendation(
  databaseName: string,
): Promise<ResourceRecommendation> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/resource-recommendation`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch resource recommendation failed: ${res.status}`),
    )
  }
  return (await res.json()) as ResourceRecommendation
}

export function databaseResourceRecommendationQueryOptions(databaseName: string) {
  return queryOptions({
    queryKey: databaseResourceRecommendationKeys.detail(databaseName),
    queryFn: () => fetchDatabaseResourceRecommendation(databaseName),
  })
}

export function useDatabaseResourceRecommendation(databaseName: string, enabled = true) {
  return useQuery({
    ...databaseResourceRecommendationQueryOptions(databaseName),
    enabled,
  })
}
