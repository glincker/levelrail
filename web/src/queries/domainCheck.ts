// Query-key factory and fetcher for GET
// /api/v1/apps/{name}/domains/{domain}/check
// (internal/api/domain_check.go's handleCheckDomain): the guidance layer
// on top of domain connection, so an operator who doesn't already know
// what an A record is can see the exact DNS record to add and watch it
// flip to "connected" once it actually resolves.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type DomainCheckStatus =
  | 'connected'
  | 'not_resolving'
  | 'resolves_elsewhere'
  | 'unconfigured'

// Mirrors internal/api/domain_check.go's domainCheckResponse wire shape
// exactly.
export interface DomainCheckResult {
  domain: string
  expected_host?: string
  host_inferred?: boolean
  resolved: boolean
  resolved_hosts?: string[]
  status: DomainCheckStatus
}

export const domainCheckKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'check'] as const,
}

export async function fetchDomainCheck(
  appName: string,
  domain: string,
): Promise<DomainCheckResult> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/check`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `check domain failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainCheckResult
}

// Polled only while the domain hasn't resolved to this control plane
// yet, the same "poll while the outcome is still in flight, then go
// idle" restraint queries/backupHistory.ts's useBackupHistory already
// applies to a different in-flight-outcome problem. DNS propagation can
// realistically take minutes to hours (unlike a database dump, which
// finishes in seconds), so unlike that precedent this auto-poll is also
// capped at MAX_AUTO_POLL_ATTEMPTS: past that, DomainDnsCheck's own
// "Check now" button is how an operator resumes checking, not a silent
// background poll running for the rest of the tab's lifetime.
const UNRESOLVED_POLL_INTERVAL_MS = 5_000
const MAX_AUTO_POLL_ATTEMPTS = 24 // 24 * 5s = 2 minutes of automatic polling

export function domainCheckQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainCheckKeys.detail(appName, domain),
    queryFn: () => fetchDomainCheck(appName, domain),
    enabled: domain.length > 0,
  })
}

export function useDomainCheck(appName: string, domain: string) {
  return useQuery({
    ...domainCheckQueryOptions(appName, domain),
    refetchInterval: (query) => {
      if (query.state.data?.status === 'connected') return false
      if (query.state.dataUpdateCount >= MAX_AUTO_POLL_ATTEMPTS) return false
      return UNRESOLVED_POLL_INTERVAL_MS
    },
  })
}
