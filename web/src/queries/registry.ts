// GET/PUT/DELETE /api/v1/settings/registry
// (internal/api/registry_settings.go's registrySettingsResource),
// following the same shape queries/cloudflareTunnel.ts already
// establishes for a single platform-wide settings row.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const registryKeys = {
  all: ['registry'] as const,
}

// RegistrySettings mirrors registrySettingsResource exactly. Password is
// write-only on the response side too, in a sense: it's populated only
// by updateRegistrySettings' own response the moment a fresh credential
// is generated (the registry's first enable), never on a plain GET.
export interface RegistrySettings {
  enabled: boolean
  host?: string
  username?: string
  has_credentials: boolean
  status: 'running' | 'stopped' | 'error'
  message?: string
  password?: string
}

export interface UpdateRegistrySettingsRequest {
  enabled: boolean
  host?: string
}

export async function fetchRegistrySettings(): Promise<RegistrySettings> {
  const res = await fetch('/api/v1/settings/registry')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch registry settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as RegistrySettings
}

export function registrySettingsQueryOptions() {
  return queryOptions({
    queryKey: registryKeys.all,
    queryFn: fetchRegistrySettings,
    staleTime: 60_000,
  })
}

export function useRegistrySettings() {
  return useSuspenseQuery(registrySettingsQueryOptions())
}

// Non-suspending variant for callers that want to read registry status as
// a supplementary signal without making an unrelated page's whole
// Suspense boundary wait on it, the same reasoning
// queries/cloudflareTunnel.ts's useCloudflareTunnelStatus already uses.
export function useRegistryStatus() {
  return useQuery(registrySettingsQueryOptions())
}

// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case queries/cloudflareTunnel.ts's
// updateCloudflareTunnelSettings carries for the identical reason.
export async function updateRegistrySettings(
  req: UpdateRegistrySettingsRequest,
): Promise<RegistrySettings> {
  const res = await fetch('/api/v1/settings/registry', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'The built-in registry requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update registry settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as RegistrySettings
}

export function useUpdateRegistrySettings() {
  const queryClient = useQueryClient()
  return useMutation<RegistrySettings, ApiError, UpdateRegistrySettingsRequest>({
    mutationFn: updateRegistrySettings,
    onSuccess: (updated) => {
      queryClient.setQueryData(registryKeys.all, updated)
    },
  })
}

// DELETE /api/v1/settings/registry: disables the registry and clears its
// generated credentials in one step.
export async function disableRegistry(): Promise<RegistrySettings> {
  const res = await fetch('/api/v1/settings/registry', { method: 'DELETE' })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'The built-in registry requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `disable registry failed: ${res.status}`),
    )
  }
  return (await res.json()) as RegistrySettings
}

export function useDisableRegistry() {
  const queryClient = useQueryClient()
  return useMutation<RegistrySettings, ApiError, void>({
    mutationFn: disableRegistry,
    onSuccess: (updated) => {
      queryClient.setQueryData(registryKeys.all, updated)
    },
  })
}
