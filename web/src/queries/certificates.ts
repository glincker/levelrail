// Query-key factory and fetcher for GET /api/v1/certificates
// (internal/api/certificates.go's handleListCertificates): the TLS
// renewal-visibility gap this project treats as its central risk
// ("a cert renewal fails silently at 3am"), previously
// nowhere in the API at all. Mirrors systemStatus.ts's own shape: an
// ordinary suspense query, requires a session, primed by the settings/
// general route's own loader.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AcmeFailure, TlsOwner } from './domainCheck'

export const certificatesKeys = {
  all: ['certificates'] as const,
}

// CertificateStatus mirrors internal/api/certificates.go's
// certificateStatus wire shape exactly. Status is one of "healthy",
// "expiring_soon", "expired": see that file's own statusForExpiry doc
// comment for why there is no fourth "renewal failed" state, this
// codebase's TLS automation only drives Caddy's offline internal issuer
// today, real ACME renewal-error tracking is a real gap but not one this
// read-only endpoint invents data for.
// CertKind says who signed the certificate a domain serves.
export type CertKind =
  'none' | 'internal' | 'staging' | 'letsencrypt' | 'custom' | 'proxy' | 'other'

export interface CertRenewal {
  at: string
  result: 'ok' | 'failed'
  error?: string
}

export interface CertificateStatus {
  domain: string
  sans?: string[]
  issuer?: string
  not_before: string
  not_after: string
  status: 'healthy' | 'expiring_soon' | 'expired'
  renewal?: 'ok' | 'stalled'
  // apps is every app or static site owning domain; empty when domain
  // matches none of them.
  apps?: string[]
  // source is "custom" for an operator-uploaded certificate
  // (setDomainTLSCert), "acme" for Caddy's automatic ACME/internal
  // issuance.
  // "proxy" is the certificate a fronting proxy serves, not a stored one.
  source: 'acme' | 'custom' | 'proxy'
  // tls_owner is who terminates TLS for this hostname.
  tls_owner?: TlsOwner
  // managed_by is 'proxy' when a fronting proxy issued and renews this
  // certificate; set client-side from the proxy integration probe.
  managed_by?: 'proxy'
  // acme_failure is the CA's last error for this hostname, when its most
  // recent issue or renewal attempt failed.
  acme_failure?: AcmeFailure
  // expiry_class is 'incident' only when this certificate's expiry needs
  // an operator; the server never sets it for internal-CA, proxy-owned or
  // unrouted certificates.
  expiry_class?: 'incident'
  // kind, days_left, last_renewal and next_retry_at are the certificate
  // truth shown as a badge: who signed it, how long it has, how the last
  // renewal went and when the watchdog tries a stuck one again.
  kind?: CertKind
  days_left?: number
  last_renewal?: CertRenewal
  next_retry_at?: string
}

export async function fetchCertificates(): Promise<CertificateStatus[]> {
  const res = await fetch('/api/v1/certificates')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch certificates failed: ${res.status}`),
    )
  }
  return (await res.json()) as CertificateStatus[]
}

// staleTime matches systemStatusQueryOptions's own reasoning: a settings
// page visited occasionally doesn't need live expiry tracking down to
// the second, a fresh read on each navigation to this route is enough.
export function certificatesQueryOptions() {
  return queryOptions({
    queryKey: certificatesKeys.all,
    queryFn: fetchCertificates,
    staleTime: 60_000,
  })
}

export function useCertificates() {
  return useSuspenseQuery(certificatesQueryOptions())
}
