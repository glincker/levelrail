// GET/PUT/DELETE /api/v1/settings/cloudflare-dns
// (internal/api/cloudflare_dns.go's cloudflareDNSResource), mirroring
// queries/cloudflareTunnel.ts's shape for a distinct credential: a
// scoped Cloudflare API token (Zone:DNS:Edit) for ACME's DNS-01
// challenge, the only challenge type that can issue a wildcard
// certificate, not the cloudflared connector token.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const cloudflareDnsKeys = {
  all: ['cloudflare-dns'] as const,
}

// CloudflareDnsSettings mirrors cloudflareDNSResource exactly. The token
// itself never appears here: write-only on the request side, has_token
// reports presence instead.
export interface CloudflareDnsSettings {
  enabled: boolean
  has_token: boolean
}

export interface UpdateCloudflareDnsRequest {
  enabled: boolean
  // Empty/omitted means "leave the currently stored token unchanged".
  token?: string
}

export async function fetchCloudflareDnsSettings(): Promise<CloudflareDnsSettings> {
  const res = await fetch('/api/v1/settings/cloudflare-dns')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch cloudflare dns settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as CloudflareDnsSettings
}

export function cloudflareDnsSettingsQueryOptions() {
  return queryOptions({
    queryKey: cloudflareDnsKeys.all,
    queryFn: fetchCloudflareDnsSettings,
    staleTime: 60_000,
  })
}

export function useCloudflareDnsSettings() {
  return useSuspenseQuery(cloudflareDnsSettingsQueryOptions())
}

// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case queries/cloudflareTunnel.ts's
// updateCloudflareTunnelSettings carries for the identical reason.
export async function updateCloudflareDnsSettings(
  req: UpdateCloudflareDnsRequest,
): Promise<CloudflareDnsSettings> {
  const res = await fetch('/api/v1/settings/cloudflare-dns', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Cloudflare DNS-01 requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update cloudflare dns settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as CloudflareDnsSettings
}

export function useUpdateCloudflareDnsSettings() {
  const queryClient = useQueryClient()
  return useMutation<
    CloudflareDnsSettings,
    ApiError,
    UpdateCloudflareDnsRequest
  >({
    mutationFn: updateCloudflareDnsSettings,
    onSuccess: (updated) => {
      queryClient.setQueryData(cloudflareDnsKeys.all, updated)
    },
  })
}

// DELETE /api/v1/settings/cloudflare-dns: disables DNS-01 and clears the
// stored token in one step. Returns the resulting resource, not 204, the
// same shape disconnectCloudflareTunnel establishes.
export async function disconnectCloudflareDns(): Promise<CloudflareDnsSettings> {
  const res = await fetch('/api/v1/settings/cloudflare-dns', {
    method: 'DELETE',
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Cloudflare DNS-01 requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `disconnect cloudflare dns failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as CloudflareDnsSettings
}

export function useDisconnectCloudflareDns() {
  const queryClient = useQueryClient()
  return useMutation<CloudflareDnsSettings, ApiError, void>({
    mutationFn: disconnectCloudflareDns,
    onSuccess: (updated) => {
      queryClient.setQueryData(cloudflareDnsKeys.all, updated)
    },
  })
}
