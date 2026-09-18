// GET/PUT/DELETE /api/v1/settings/vault (internal/api/vault_settings.go's
// vaultSettingsResource), following the same shape queries/
// cloudflareTunnel.ts already establishes for a single platform-wide
// settings row.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const vaultKeys = {
  all: ['vault-settings'] as const,
}

// VaultSettings mirrors vaultSettingsResource exactly. The credential
// (a Vault token or AppRole secret ID, depending on auth_method) never
// appears here: it's write-only on the request side (see
// UpdateVaultSettingsRequest), has_credential reports presence instead.
export interface VaultSettings {
  enabled: boolean
  address: string
  auth_method: 'token' | 'approle'
  namespace?: string
  role_id?: string
  mount_path: string
  has_credential: boolean
}

export interface UpdateVaultSettingsRequest {
  enabled: boolean
  address: string
  auth_method: 'token' | 'approle'
  namespace?: string
  role_id?: string
  mount_path?: string
  // Empty/omitted means "leave the currently stored credential
  // unchanged".
  credential?: string
}

export async function fetchVaultSettings(): Promise<VaultSettings> {
  const res = await fetch('/api/v1/settings/vault')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch vault settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as VaultSettings
}

export function vaultSettingsQueryOptions() {
  return queryOptions({
    queryKey: vaultKeys.all,
    queryFn: fetchVaultSettings,
    staleTime: 60_000,
  })
}

export function useVaultSettings() {
  return useSuspenseQuery(vaultSettingsQueryOptions())
}

// Non-suspending variant, the same reasoning
// queries/cloudflareTunnel.ts's useCloudflareTunnelStatus already gives
// for a comparable optional guidance signal (e.g. a setup checklist that
// shouldn't block its whole page on this one settings row).
export function useVaultStatus() {
  return useQuery(vaultSettingsQueryOptions())
}

// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case queries/cloudflareTunnel.ts's
// updateCloudflareTunnelSettings carries for the identical reason (both
// route through internal/secrets for the credential).
export async function updateVaultSettings(
  req: UpdateVaultSettingsRequest,
): Promise<VaultSettings> {
  const res = await fetch('/api/v1/settings/vault', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Vault requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update vault settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as VaultSettings
}

export function useUpdateVaultSettings() {
  const queryClient = useQueryClient()
  return useMutation<VaultSettings, ApiError, UpdateVaultSettingsRequest>({
    mutationFn: updateVaultSettings,
    onSuccess: (updated) => {
      queryClient.setQueryData(vaultKeys.all, updated)
    },
  })
}

// DELETE /api/v1/settings/vault: disables vault and clears the stored
// credential in one step.
export async function disconnectVault(): Promise<VaultSettings> {
  const res = await fetch('/api/v1/settings/vault', { method: 'DELETE' })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Vault requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `disconnect vault failed: ${res.status}`),
    )
  }
  return (await res.json()) as VaultSettings
}

export function useDisconnectVault() {
  const queryClient = useQueryClient()
  return useMutation<VaultSettings, ApiError, void>({
    mutationFn: disconnectVault,
    onSuccess: (updated) => {
      queryClient.setQueryData(vaultKeys.all, updated)
    },
  })
}
