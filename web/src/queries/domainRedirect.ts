// Query-key factory and fetcher for GET/PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/redirect
// (internal/api/domain_redirect.go's domainRedirectResource): an opt-in
// redirect to a target URL for one of an app's domains, enforced by the
// embedded Caddy ingress on the next reconcile pass. Mirrors
// queries/domainWaf.ts's shape: this resource carries real fields
// (target URL, status code), so set takes a full request body.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api's domainRedirectResource wire shape exactly.
export interface DomainRedirect {
  domain: string
  enabled: boolean
  target_url?: string
  status_code: number
}

// Mirrors internal/api's setDomainRedirectRequest wire shape exactly.
export interface SetDomainRedirectRequest {
  target_url: string
  status_code?: number
}

export const domainRedirectKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'redirect'] as const,
}

function redirectPath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/redirect`
}

export async function fetchDomainRedirect(
  appName: string,
  domain: string,
): Promise<DomainRedirect> {
  const res = await fetch(redirectPath(appName, domain))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch domain redirect failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainRedirect
}

export function domainRedirectQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainRedirectKeys.detail(appName, domain),
    queryFn: () => fetchDomainRedirect(appName, domain),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainRedirect(appName: string, domain: string) {
  return useQuery(domainRedirectQueryOptions(appName, domain))
}

async function setDomainRedirect(
  appName: string,
  domain: string,
  req: SetDomainRedirectRequest,
): Promise<DomainRedirect> {
  const res = await fetch(redirectPath(appName, domain), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set domain redirect failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainRedirect
}

export function useSetDomainRedirect(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainRedirect, ApiError, SetDomainRedirectRequest>({
    mutationFn: (req) => setDomainRedirect(appName, domain, req),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainRedirectKeys.detail(appName, domain),
        updated,
      )
    },
  })
}

async function clearDomainRedirect(
  appName: string,
  domain: string,
): Promise<DomainRedirect> {
  const res = await fetch(redirectPath(appName, domain), { method: 'DELETE' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `clear domain redirect failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainRedirect
}

export function useClearDomainRedirect(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainRedirect, ApiError, void>({
    mutationFn: () => clearDomainRedirect(appName, domain),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainRedirectKeys.detail(appName, domain),
        updated,
      )
    },
  })
}
