// Fetchers for the slow request investigation (GET
// /api/v1/apps/{name}/investigate) and the crashloop or failed deploy
// context (GET /api/v1/apps/{name}/failure-context).

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { FailureContext, InvestigateResponse } from '../types/investigate'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const investigateKeys = {
  window: (appName: string, fromIso: string, toIso: string) =>
    [...appKeys.detail(appName), 'investigate', fromIso, toIso] as const,
  failure: (appName: string) =>
    [...appKeys.detail(appName), 'failure-context'] as const,
}

export const FAILURE_CONTEXT_POLL_MS = 15_000

async function getJSON<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function investigateQueryOptions(appName: string, from: Date, to: Date) {
  const fromIso = from.toISOString()
  const toIso = to.toISOString()
  return queryOptions({
    queryKey: investigateKeys.window(appName, fromIso, toIso),
    queryFn: () => {
      const q = new URLSearchParams({ from: fromIso, to: toIso })
      return getJSON<InvestigateResponse>(
        `/api/v1/apps/${encodeURIComponent(appName)}/investigate?${q.toString()}`,
        'investigate',
      )
    },
  })
}

export function useInvestigation(
  appName: string,
  from: Date | null,
  to: Date | null,
) {
  const enabled = from !== null && to !== null
  return useQuery({
    ...investigateQueryOptions(appName, from ?? new Date(0), to ?? new Date(0)),
    enabled,
  })
}

export function failureContextQueryOptions(appName: string) {
  return queryOptions({
    queryKey: investigateKeys.failure(appName),
    queryFn: () =>
      getJSON<FailureContext>(
        `/api/v1/apps/${encodeURIComponent(appName)}/failure-context`,
        'failure context',
      ),
    refetchInterval: FAILURE_CONTEXT_POLL_MS,
  })
}

export function useFailureContext(appName: string) {
  return useQuery(failureContextQueryOptions(appName))
}
