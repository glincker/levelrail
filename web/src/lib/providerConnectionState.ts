import type {
  NodeProviderResource,
  NodeProviderStatus,
} from '../types/nodeProvision'

// Single source of truth for "is this cloud provider connected": the API's
// account-aware flag, falling back to the legacy credential for older servers.
export function isProviderConnected(r: NodeProviderResource): boolean {
  return r.connected ?? r.has_token
}

export function providerAccounts(r: NodeProviderResource) {
  return r.accounts ?? []
}

// A failed legacy credential test only shows when no account carries the provider.
export function providerStatus(r: NodeProviderResource): NodeProviderStatus {
  const accounts = providerAccounts(r)
  const activeAccount = accounts.some(
    (a) => a.status === 'active' && !a.needs_relogin,
  )
  if (r.status === 'test_failed' && !activeAccount) return 'test_failed'
  return isProviderConnected(r) ? 'connected' : 'not_connected'
}
