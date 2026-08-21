// Query-key factories and fetchers for the centralized Domains page:
// GET /api/v1/domains (internal/api/ingress_settings.go's
// handleListDomains, every service_domains row) and GET/PUT
// /api/v1/settings/ingress (handleGetIngressSettings/
// handleUpdateIngressSettings, the platform-wide ACME toggle and
// primary-domain setting). Two distinct resources sharing one file since
// they're consumed by the same route and nowhere else, the same
// "one file per route's own data needs" shape queries/certificates.ts
// already keeps separate from queries/apps.ts despite both feeding
// settings/general.tsx.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DomainCheckStatus } from './domainCheck'

export const domainKeys = {
  all: ['domains'] as const,
}

// Domain mirrors internal/api/ingress_settings.go's domainResource wire
// shape exactly.
export interface Domain {
  domain: string
  service_name: string
}

export async function fetchDomains(): Promise<Domain[]> {
  const res = await fetch('/api/v1/domains')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch domains failed: ${res.status}`),
    )
  }
  return (await res.json()) as Domain[]
}

// staleTime matches certificatesQueryOptions's own reasoning
// (queries/certificates.ts): a settings-style page visited occasionally
// doesn't need live tracking down to the second.
export function domainsQueryOptions() {
  return queryOptions({
    queryKey: domainKeys.all,
    queryFn: fetchDomains,
    staleTime: 60_000,
  })
}

export function useDomains() {
  return useSuspenseQuery(domainsQueryOptions())
}

export const ingressSettingsKeys = {
  all: ['ingress-settings'] as const,
}

// IngressSettings mirrors internal/api/ingress_settings.go's
// ingressSettingsResource wire shape exactly.
export interface IngressSettings {
  primary_domain?: string
  acme_enabled: boolean
  acme_email?: string
  acme_directory_url?: string
}

export async function fetchIngressSettings(): Promise<IngressSettings> {
  const res = await fetch('/api/v1/settings/ingress')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch ingress settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as IngressSettings
}

export function ingressSettingsQueryOptions() {
  return queryOptions({
    queryKey: ingressSettingsKeys.all,
    queryFn: fetchIngressSettings,
    staleTime: 60_000,
  })
}

export function useIngressSettings() {
  return useSuspenseQuery(ingressSettingsQueryOptions())
}

// PUT /api/v1/settings/ingress (handleUpdateIngressSettings),
// AbilityRoot-gated on the backend: rejects acme_enabled without a
// syntactically valid acme_email with a 400, whose message
// readErrorMessage surfaces verbatim so the settings form can show the
// real server-side reason rather than a generic failure.
export async function updateIngressSettings(
  settings: IngressSettings,
): Promise<IngressSettings> {
  const res = await fetch('/api/v1/settings/ingress', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update ingress settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as IngressSettings
}

// On success, writes the server's response straight into the cache
// (the PUT response already is the new canonical state), the same
// pattern useUpdateApp (queries/apps.ts) already establishes, rather
// than invalidating and refetching.
export function useUpdateIngressSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateIngressSettings,
    onSuccess: (updated) => {
      queryClient.setQueryData(ingressSettingsKeys.all, updated)
    },
  })
}

export const ingressDomainCheckKeys = {
  all: ['ingress-settings', 'check'] as const,
}

// IngressDomainCheckResult mirrors GET /api/v1/settings/ingress/check's
// (internal/api/ingress_settings.go's handleCheckIngressDomain) wire
// shape exactly: domainCheckResponse's own fields plus configured,
// discriminated on it so a caller can't read Status/ExpectedHost without
// first narrowing past the "no primary domain set yet" case.
export type IngressDomainCheckResult =
  | { configured: false }
  | {
      configured: true
      domain: string
      expected_host?: string
      expected_ipv4?: string[]
      expected_ipv6?: string[]
      host_inferred?: boolean
      resolved: boolean
      resolved_hosts?: string[]
      status: DomainCheckStatus
    }

export async function fetchIngressDomainCheck(): Promise<IngressDomainCheckResult> {
  const res = await fetch('/api/v1/settings/ingress/check')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `check ingress domain failed: ${res.status}`),
    )
  }
  return (await res.json()) as IngressDomainCheckResult
}

export function ingressDomainCheckQueryOptions() {
  return queryOptions({
    queryKey: ingressDomainCheckKeys.all,
    queryFn: fetchIngressDomainCheck,
  })
}

// Same bounded auto-poll restraint as queries/domainCheck.ts's
// useDomainCheck: keep polling only while the primary domain hasn't
// resolved to this control plane yet, capped so an idle open settings
// tab can't turn into an indefinite background DNS poll.
const UNRESOLVED_POLL_INTERVAL_MS = 5_000
const MAX_AUTO_POLL_ATTEMPTS = 24 // 24 * 5s = 2 minutes of automatic polling

export function useIngressDomainCheck() {
  return useQuery({
    ...ingressDomainCheckQueryOptions(),
    refetchInterval: (query) => {
      if (!query.state.data?.configured) return false
      if (query.state.data.status === 'connected') return false
      if (query.state.dataUpdateCount >= MAX_AUTO_POLL_ATTEMPTS) return false
      return UNRESOLVED_POLL_INTERVAL_MS
    },
  })
}
