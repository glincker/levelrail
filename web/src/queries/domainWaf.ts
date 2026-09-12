// Query-key factory and fetcher for GET/PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/waf
// (internal/api/domain_waf.go's domainWAFResource): opt-in WAF and
// rate-limit settings for one of an app's domains, enforced by the
// embedded Caddy ingress on the next reconcile pass. Mirrors
// queries/domainMaintenance.ts's shape, minus the "presence-only"
// toggle: this resource carries a few real fields (mode, rps, burst),
// so set takes a full request body instead of no body at all.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api's domainWAFResource wire shape exactly.
export interface DomainWaf {
  domain: string
  waf_enabled: boolean
  waf_mode: 'detect' | 'block'
  rate_limit_enabled: boolean
  rate_limit_rps: number
  rate_limit_burst: number
}

// Mirrors internal/api's setDomainWAFRequest wire shape exactly.
export interface SetDomainWafRequest {
  waf_enabled: boolean
  waf_mode?: 'detect' | 'block'
  rate_limit_rps: number
  rate_limit_burst: number
}

export const domainWafKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'waf'] as const,
}

function wafPath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/waf`
}

export async function fetchDomainWaf(appName: string, domain: string): Promise<DomainWaf> {
  const res = await fetch(wafPath(appName, domain))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch domain waf failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainWaf
}

export function domainWafQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainWafKeys.detail(appName, domain),
    queryFn: () => fetchDomainWaf(appName, domain),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainWaf(appName: string, domain: string) {
  return useQuery(domainWafQueryOptions(appName, domain))
}

async function setDomainWaf(
  appName: string,
  domain: string,
  req: SetDomainWafRequest,
): Promise<DomainWaf> {
  const res = await fetch(wafPath(appName, domain), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set domain waf failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainWaf
}

export function useSetDomainWaf(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainWaf, ApiError, SetDomainWafRequest>({
    mutationFn: (req) => setDomainWaf(appName, domain, req),
    onSuccess: (updated) => {
      queryClient.setQueryData(domainWafKeys.detail(appName, domain), updated)
    },
  })
}

async function clearDomainWaf(appName: string, domain: string): Promise<DomainWaf> {
  const res = await fetch(wafPath(appName, domain), { method: 'DELETE' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clear domain waf failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainWaf
}

export function useClearDomainWaf(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainWaf, ApiError, void>({
    mutationFn: () => clearDomainWaf(appName, domain),
    onSuccess: (updated) => {
      queryClient.setQueryData(domainWafKeys.detail(appName, domain), updated)
    },
  })
}
