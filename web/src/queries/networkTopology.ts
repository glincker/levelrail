// Query-key factory and fetcher for GET /api/v1/network/topology
// (internal/api/network_topology.go's handleGetNetworkTopology),
// mirroring queries/nodes.ts's own shape. AbilityRead-gated (unlike most
// of queries/nodes.ts, which is AbilityRoot-gated), so this succeeds for
// any authenticated session, not just root.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import type { NetworkTopologyResponse } from '../types/networkTopology'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const networkTopologyKeys = {
  all: ['network-topology'] as const,
}

export async function fetchNetworkTopology(): Promise<NetworkTopologyResponse> {
  const res = await fetch('/api/v1/network/topology')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch network topology failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NetworkTopologyResponse
}

export function networkTopologyQueryOptions() {
  return queryOptions({
    queryKey: networkTopologyKeys.all,
    queryFn: fetchNetworkTopology,
  })
}

export function useNetworkTopology() {
  return useSuspenseQuery(networkTopologyQueryOptions())
}
