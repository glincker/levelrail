// Query-key factory and fetcher for GET /api/v1/network/proxy
// (internal/api/network_proxy.go's handleGetNetworkProxy): a per-domain
// reachability join for the Traffic page. AbilityRead-gated, same as
// queries/networkTopology.ts, so this succeeds for any authenticated
// session, not just root.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// NetworkProxyDomain mirrors internal/api/network_proxy.go's
// networkProxyDomainResource wire shape exactly.
export interface NetworkProxyDomain {
  domain: string
  app: string
  node_id: string
  node_name?: string
  is_local_node: boolean
  port: number
  reachable: boolean
  reason?: string
  fix_command?: string
  tls_status?: 'healthy' | 'expiring_soon' | 'expired'
  tls_issuer?: string
  tls_source?: 'acme' | 'custom'
}

export interface NetworkProxyResponse {
  domains: NetworkProxyDomain[]
}

export const networkProxyKeys = {
  all: ['network-proxy'] as const,
}

export async function fetchNetworkProxy(): Promise<NetworkProxyResponse> {
  const res = await fetch('/api/v1/network/proxy')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch network proxy failed: ${res.status}`),
    )
  }
  return (await res.json()) as NetworkProxyResponse
}

export function networkProxyQueryOptions() {
  return queryOptions({
    queryKey: networkProxyKeys.all,
    queryFn: fetchNetworkProxy,
  })
}

export function useNetworkProxy() {
  return useSuspenseQuery(networkProxyQueryOptions())
}
