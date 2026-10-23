// Queries and mutations for /api/v1/resource-advisor and the beta switch.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { useExperimentalFeatures } from '../hooks/useExperimental'
import { isFeatureVisible } from '../lib/experimental'
import { useEnvironmentScope } from '../lib/environmentScope'
import { currentScope, scopeKey, scopeUrl } from '../lib/scopeQuery'
import { devModeKeys } from './devMode'
import { appKeys } from './apps'
import { databaseKeys } from './databases'
import type {
  AdvisorApplyResult,
  AdvisorFinding,
  AdvisorFindings,
  AdvisorSummary,
  BetaFeature,
} from '../types/resourceAdvisor'

export const advisorKeys = {
  all: ['resource-advisor'] as const,
  findings: (resource: string, includeHidden: boolean) =>
    [...advisorKeys.all, 'findings', resource, includeHidden] as const,
  summary: (scope: readonly unknown[] = []) =>
    [...advisorKeys.all, 'summary', ...scope] as const,
  preview: (id: string) => [...advisorKeys.all, 'preview', id] as const,
}

export const betaFeatureKeys = {
  all: ['beta-features'] as const,
}

async function request<T>(
  url: string,
  init: RequestInit,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function post(body: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function advisorFindingsQueryOptions(
  resource = '',
  includeHidden = false,
) {
  const params = new URLSearchParams()
  if (resource) params.set('resource', resource)
  if (includeHidden) params.set('include', 'hidden')
  const qs = params.toString()
  return queryOptions({
    queryKey: advisorKeys.findings(resource, includeHidden),
    queryFn: () =>
      request<AdvisorFindings>(
        `/api/v1/resource-advisor/findings${qs ? `?${qs}` : ''}`,
        {},
        'fetch resource advice',
      ),
    staleTime: 30_000,
  })
}

export function advisorSummaryQueryOptions() {
  const scope = currentScope()
  return queryOptions({
    queryKey: advisorKeys.summary(scopeKey(scope)),
    queryFn: () =>
      request<AdvisorSummary>(
        scopeUrl('/api/v1/resource-advisor/summary', scope),
        {},
        'fetch resource advice summary',
      ),
    staleTime: 60_000,
  })
}

// True when the control plane has the beta switched on, so callers can skip
// requests that would 404 while it is off.
export function useResourceAdvisorEnabled(): boolean {
  return isFeatureVisible('resource-advisor', useExperimentalFeatures())
}

export function useAdvisorFindings(resource = '', includeHidden = false) {
  const enabled = useResourceAdvisorEnabled()
  return useQuery({
    ...advisorFindingsQueryOptions(resource, includeHidden),
    enabled,
  })
}

export function useAdvisorSummary() {
  useEnvironmentScope()
  const enabled = useResourceAdvisorEnabled()
  return useQuery({ ...advisorSummaryQueryOptions(), enabled })
}

export function fetchApplyPreview(id: string): Promise<AdvisorApplyResult> {
  return request<AdvisorApplyResult>(
    `/api/v1/resource-advisor/findings/${encodeURIComponent(id)}/apply`,
    post({ confirm: false }),
    'preview resource advice',
  )
}

export function useApplyPreview(id: string, enabled: boolean) {
  return useQuery({
    queryKey: advisorKeys.preview(id),
    queryFn: () => fetchApplyPreview(id),
    enabled,
    staleTime: 0,
    gcTime: 0,
  })
}

export function useApplyFinding() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<AdvisorApplyResult>(
        `/api/v1/resource-advisor/findings/${encodeURIComponent(id)}/apply`,
        post({ confirm: true }),
        'apply resource advice',
      ),
    onSuccess: (res) => {
      void qc.invalidateQueries({ queryKey: advisorKeys.all })
      void qc.invalidateQueries({
        queryKey:
          res.resource.kind === 'app'
            ? appKeys.detail(res.resource.name)
            : databaseKeys.detail(res.resource.name),
      })
    },
  })
}

export type FindingStateVerb = 'dismiss' | 'snooze' | 'restore'

export function useFindingState() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      verb,
      duration,
    }: {
      id: string
      verb: FindingStateVerb
      duration?: string
    }) =>
      request<AdvisorFinding>(
        `/api/v1/resource-advisor/findings/${encodeURIComponent(id)}/${verb}`,
        post(duration ? { duration } : {}),
        `${verb} resource advice`,
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: advisorKeys.all })
    },
  })
}

export function betaFeaturesQueryOptions() {
  return queryOptions({
    queryKey: betaFeatureKeys.all,
    queryFn: () =>
      request<BetaFeature[]>(
        '/api/v1/settings/beta-features',
        {},
        'fetch beta features',
      ),
  })
}

export function useBetaFeatures() {
  return useQuery(betaFeaturesQueryOptions())
}

export function useSetBetaFeature() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ feature, enabled }: { feature: string; enabled: boolean }) =>
      request<BetaFeature>(
        `/api/v1/settings/beta-features/${encodeURIComponent(feature)}`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled }),
        },
        'switch beta feature',
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: betaFeatureKeys.all })
      void qc.invalidateQueries({ queryKey: devModeKeys.all })
      void qc.invalidateQueries({ queryKey: advisorKeys.all })
    },
  })
}
