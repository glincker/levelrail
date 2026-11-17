import { ApiError } from './apiError'

// Wire shape of GET /api/v1/node-providers/{provider}/capabilities.
export type ProviderCapabilityState = 'granted' | 'missing' | 'unknown'

export interface ProviderCapability {
  id: string
  group: string
  state: ProviderCapabilityState
  evidence: string
  needs: string
  used_by: string[] | null
  observed_at: string
}

export interface ProviderCapabilities {
  provider: string
  account_id: string
  method: string
  checked_at: string
  cached: boolean
  next_check_at?: string
  live_check_error?: string
  capabilities: ProviderCapability[]
}

// Missing means a probe got 401 or 403; unknown or absent is not a block.
export function needsScope(
  caps: ProviderCapabilities | undefined,
  id: string,
): string | null {
  const hit = caps?.capabilities.find((c) => c.id === id)
  return hit?.state === 'missing' ? hit.needs || id : null
}

export function capabilitySummary(caps: ProviderCapabilities): {
  granted: number
  total: number
} {
  return {
    granted: caps.capabilities.filter((c) => c.state === 'granted').length,
    total: caps.capabilities.length,
  }
}

export const PROVIDER_SCOPE_MISSING = 'provider_scope_missing'

// A 424 from any provider action: the credential works but lacks one scope.
export function isProviderScopeMissing(error: unknown): error is ApiError {
  return error instanceof ApiError && error.code === PROVIDER_SCOPE_MISSING
}
