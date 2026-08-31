// Query-key factory and fetcher for GET /api/v1/apps/{name}/diagnose
// (internal/api/diagnose.go's handleDiagnoseApp). Unlike
// queries/deploys.ts and queries/deployAttempts.ts, this is fetched
// lazily (plain useQuery with `enabled`, not useSuspenseQuery): most
// apps most of the time have nothing to diagnose, so this should never
// fire on a normal page load, only when DiagnosisPanel actually renders
// (a failed attempt or a crashlooping app) or an operator expands it.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { Diagnosis } from '../types/diagnosis'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const diagnosisKeys = {
  detail: (appName: string, deployId?: string) =>
    [...appKeys.detail(appName), 'diagnosis', deployId ?? 'latest'] as const,
}

export async function fetchDiagnosis(
  appName: string,
  deployId?: string,
): Promise<Diagnosis> {
  let url = `/api/v1/apps/${encodeURIComponent(appName)}/diagnose`
  if (deployId) {
    url += `?deploy_id=${encodeURIComponent(deployId)}`
  }
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch diagnosis failed: ${res.status}`),
    )
  }
  return (await res.json()) as Diagnosis
}

export function diagnosisQueryOptions(appName: string, deployId?: string) {
  return queryOptions({
    queryKey: diagnosisKeys.detail(appName, deployId),
    queryFn: () => fetchDiagnosis(appName, deployId),
  })
}

export function useDiagnosis(
  appName: string,
  deployId: string | undefined,
  enabled: boolean,
) {
  return useQuery({ ...diagnosisQueryOptions(appName, deployId), enabled })
}
