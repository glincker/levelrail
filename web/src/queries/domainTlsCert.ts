// Query-key factory and fetcher for GET/PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/tls-cert
// (internal/api/domain_tls_cert.go's domainTLSCertResource): an
// operator-supplied (BYO) TLS certificate for one of an app's domains,
// used by the embedded Caddy ingress in place of automatic ACME/
// internal issuance on the next reconcile pass. Mirrors
// queries/domainBasicAuth.ts's shape: the certificate and private key
// themselves never appear here in either direction.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api's domainTLSCertResource wire shape exactly.
export interface DomainTLSCert {
  domain: string
  enabled: boolean
  uploaded_at?: string
  expires_at?: string
}

export interface SetDomainTLSCertRequest {
  cert: string
  key: string
}

export const domainTLSCertKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'tls-cert'] as const,
}

function tlsCertPath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/tls-cert`
}

export async function fetchDomainTLSCert(
  appName: string,
  domain: string,
): Promise<DomainTLSCert> {
  const res = await fetch(tlsCertPath(appName, domain))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch domain tls cert failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainTLSCert
}

export function domainTLSCertQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainTLSCertKeys.detail(appName, domain),
    queryFn: () => fetchDomainTLSCert(appName, domain),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainTLSCert(appName: string, domain: string) {
  return useQuery(domainTLSCertQueryOptions(appName, domain))
}

// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case queries/domainBasicAuth.ts's
// setDomainBasicAuth carries for the identical reason (both route
// through internal/secrets).
export async function setDomainTLSCert(
  appName: string,
  domain: string,
  req: SetDomainTLSCertRequest,
): Promise<DomainTLSCert> {
  const res = await fetch(tlsCertPath(appName, domain), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Domain TLS certificate upload requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set domain tls cert failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainTLSCert
}

export function useSetDomainTLSCert(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainTLSCert, ApiError, SetDomainTLSCertRequest>({
    mutationFn: (req) => setDomainTLSCert(appName, domain, req),
    onSuccess: (updated) => {
      queryClient.setQueryData(domainTLSCertKeys.detail(appName, domain), updated)
    },
  })
}

export async function clearDomainTLSCert(
  appName: string,
  domain: string,
): Promise<DomainTLSCert> {
  const res = await fetch(tlsCertPath(appName, domain), { method: 'DELETE' })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Domain TLS certificate upload requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clear domain tls cert failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainTLSCert
}

export function useClearDomainTLSCert(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainTLSCert, ApiError, void>({
    mutationFn: () => clearDomainTLSCert(appName, domain),
    onSuccess: (updated) => {
      queryClient.setQueryData(domainTLSCertKeys.detail(appName, domain), updated)
    },
  })
}
