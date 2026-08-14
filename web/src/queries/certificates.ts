// Query-key factory and fetcher for GET /api/v1/certificates
// (internal/api/certificates.go's handleListCertificates): the TLS
// renewal-visibility gap CLAUDE.md section 10 names as this project's
// central risk ("a cert renewal fails silently at 3am"), previously
// nowhere in the API at all. Mirrors systemStatus.ts's own shape: an
// ordinary suspense query, requires a session, primed by the settings/
// general route's own loader.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

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
export interface CertificateStatus {
  domain: string
  sans?: string[]
  issuer?: string
  not_before: string
  not_after: string
  status: 'healthy' | 'expiring_soon' | 'expired'
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
