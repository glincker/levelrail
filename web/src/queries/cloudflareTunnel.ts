// GET/PUT/DELETE /api/v1/settings/cloudflare-tunnel
// (internal/api/cloudflare_tunnel.go's cloudflareTunnelResource),
// following the same shape queries/emailSettings.ts already establishes
// for a single platform-wide settings row.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const cloudflareTunnelKeys = {
  all: ['cloudflare-tunnel'] as const,
}

// CloudflareTunnelSettings mirrors cloudflareTunnelResource exactly. The
// token itself never appears here: it's write-only on the request side
// (see UpdateCloudflareTunnelRequest), has_token reports presence
// instead.
export interface CloudflareTunnelSettings {
  enabled: boolean
  has_token: boolean
  status: 'connected' | 'disconnected' | 'error'
  message?: string
}

export interface UpdateCloudflareTunnelRequest {
  enabled: boolean
  // Empty/omitted means "leave the currently stored token unchanged".
  token?: string
}

export async function fetchCloudflareTunnelSettings(): Promise<CloudflareTunnelSettings> {
  const res = await fetch('/api/v1/settings/cloudflare-tunnel')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch cloudflare tunnel settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as CloudflareTunnelSettings
}

export function cloudflareTunnelSettingsQueryOptions() {
  return queryOptions({
    queryKey: cloudflareTunnelKeys.all,
    queryFn: fetchCloudflareTunnelSettings,
    staleTime: 60_000,
  })
}

export function useCloudflareTunnelSettings() {
  return useSuspenseQuery(cloudflareTunnelSettingsQueryOptions())
}

// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case queries/emailSettings.ts's
// updateEmailSettings carries for the identical reason (both route
// through internal/secrets).
export async function updateCloudflareTunnelSettings(
  req: UpdateCloudflareTunnelRequest,
): Promise<CloudflareTunnelSettings> {
  const res = await fetch('/api/v1/settings/cloudflare-tunnel', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Cloudflare Tunnel requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update cloudflare tunnel settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as CloudflareTunnelSettings
}

export function useUpdateCloudflareTunnelSettings() {
  const queryClient = useQueryClient()
  return useMutation<
    CloudflareTunnelSettings,
    ApiError,
    UpdateCloudflareTunnelRequest
  >({
    mutationFn: updateCloudflareTunnelSettings,
    onSuccess: (updated) => {
      queryClient.setQueryData(cloudflareTunnelKeys.all, updated)
    },
  })
}

// DELETE /api/v1/settings/cloudflare-tunnel: disables the tunnel and
// clears the stored token in one step (handleDisconnectCloudflareTunnel's
// own doc comment). Returns the resulting resource, not 204: the UI
// reflects the cleared state immediately without a refetch.
export async function disconnectCloudflareTunnel(): Promise<CloudflareTunnelSettings> {
  const res = await fetch('/api/v1/settings/cloudflare-tunnel', {
    method: 'DELETE',
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Cloudflare Tunnel requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `disconnect cloudflare tunnel failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as CloudflareTunnelSettings
}

export function useDisconnectCloudflareTunnel() {
  const queryClient = useQueryClient()
  return useMutation<CloudflareTunnelSettings, ApiError, void>({
    mutationFn: disconnectCloudflareTunnel,
    onSuccess: (updated) => {
      queryClient.setQueryData(cloudflareTunnelKeys.all, updated)
    },
  })
}
