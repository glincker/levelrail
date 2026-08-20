// Query-key factory and fetcher for GET/PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/auth
// (internal/api/domain_basic_auth.go's domainBasicAuthResource): HTTP
// Basic Auth protection for one of an app's domains, enforced by the
// embedded Caddy ingress on the next reconcile pass. Mirrors
// queries/cloudflareTunnel.ts's has_token-shaped pattern: the password
// itself never appears here in either direction.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api's domainBasicAuthResource wire shape exactly.
export interface DomainBasicAuth {
  domain: string
  enabled: boolean
  username?: string
  has_password: boolean
}

export interface SetDomainBasicAuthRequest {
  username: string
  // Empty/omitted means "leave the currently stored password unchanged".
  password?: string
}

export const domainBasicAuthKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'auth'] as const,
}

function authPath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/auth`
}

export async function fetchDomainBasicAuth(
  appName: string,
  domain: string,
): Promise<DomainBasicAuth> {
  const res = await fetch(authPath(appName, domain))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch domain basic auth failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainBasicAuth
}

export function domainBasicAuthQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainBasicAuthKeys.detail(appName, domain),
    queryFn: () => fetchDomainBasicAuth(appName, domain),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainBasicAuth(appName: string, domain: string) {
  return useQuery(domainBasicAuthQueryOptions(appName, domain))
}

// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case queries/cloudflareTunnel.ts's
// updateCloudflareTunnelSettings carries for the identical reason (both
// route through internal/secrets).
export async function setDomainBasicAuth(
  appName: string,
  domain: string,
  req: SetDomainBasicAuthRequest,
): Promise<DomainBasicAuth> {
  const res = await fetch(authPath(appName, domain), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Domain basic auth requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set domain basic auth failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainBasicAuth
}

export function useSetDomainBasicAuth(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainBasicAuth, ApiError, SetDomainBasicAuthRequest>({
    mutationFn: (req) => setDomainBasicAuth(appName, domain, req),
    onSuccess: (updated) => {
      queryClient.setQueryData(domainBasicAuthKeys.detail(appName, domain), updated)
    },
  })
}

export async function clearDomainBasicAuth(
  appName: string,
  domain: string,
): Promise<DomainBasicAuth> {
  const res = await fetch(authPath(appName, domain), { method: 'DELETE' })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Domain basic auth requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clear domain basic auth failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainBasicAuth
}

export function useClearDomainBasicAuth(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainBasicAuth, ApiError, void>({
    mutationFn: () => clearDomainBasicAuth(appName, domain),
    onSuccess: (updated) => {
      queryClient.setQueryData(domainBasicAuthKeys.detail(appName, domain), updated)
    },
  })
}
