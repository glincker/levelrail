// GET/PUT/DELETE /api/v1/settings/route53-dns
// (internal/api/route53_dns.go's route53DNSResource), mirroring
// queries/cloudflareDns.ts's shape for a second, independent ACME
// DNS-01 provider: an AWS IAM access key pair instead of a single API
// token, plus two optional non-secret fields (region, hosted zone id).

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const route53DnsKeys = {
  all: ['route53-dns'] as const,
}

// Route53DnsSettings mirrors route53DNSResource exactly. Neither
// credential half ever appears here: write-only on the request side,
// has_access_key_id/has_secret_access_key report presence instead.
export interface Route53DnsSettings {
  enabled: boolean
  region?: string
  hosted_zone_id?: string
  has_access_key_id: boolean
  has_secret_access_key: boolean
}

export interface UpdateRoute53DnsRequest {
  enabled: boolean
  region?: string
  hosted_zone_id?: string
  // Empty/omitted means "leave the currently stored credential
  // unchanged"; the two are set together or not at all.
  access_key_id?: string
  secret_access_key?: string
}

export async function fetchRoute53DnsSettings(): Promise<Route53DnsSettings> {
  const res = await fetch('/api/v1/settings/route53-dns')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch route53 dns settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as Route53DnsSettings
}

export function route53DnsSettingsQueryOptions() {
  return queryOptions({
    queryKey: route53DnsKeys.all,
    queryFn: fetchRoute53DnsSettings,
    staleTime: 60_000,
  })
}

export function useRoute53DnsSettings() {
  return useSuspenseQuery(route53DnsSettingsQueryOptions())
}

// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case queries/cloudflareDns.ts's
// updateCloudflareDnsSettings carries for the identical reason.
export async function updateRoute53DnsSettings(
  req: UpdateRoute53DnsRequest,
): Promise<Route53DnsSettings> {
  const res = await fetch('/api/v1/settings/route53-dns', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Route53 DNS-01 requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update route53 dns settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as Route53DnsSettings
}

export function useUpdateRoute53DnsSettings() {
  const queryClient = useQueryClient()
  return useMutation<Route53DnsSettings, ApiError, UpdateRoute53DnsRequest>({
    mutationFn: updateRoute53DnsSettings,
    onSuccess: (updated) => {
      queryClient.setQueryData(route53DnsKeys.all, updated)
    },
  })
}

// DELETE /api/v1/settings/route53-dns: disables DNS-01 and clears the
// stored credential pair in one step. Returns the resulting resource,
// not 204, the same shape disconnectCloudflareDns establishes.
export async function disconnectRoute53Dns(): Promise<Route53DnsSettings> {
  const res = await fetch('/api/v1/settings/route53-dns', {
    method: 'DELETE',
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Route53 DNS-01 requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `disconnect route53 dns failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as Route53DnsSettings
}

export function useDisconnectRoute53Dns() {
  const queryClient = useQueryClient()
  return useMutation<Route53DnsSettings, ApiError, void>({
    mutationFn: disconnectRoute53Dns,
    onSuccess: (updated) => {
      queryClient.setQueryData(route53DnsKeys.all, updated)
    },
  })
}
