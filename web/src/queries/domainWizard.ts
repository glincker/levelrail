// Fetchers for the domain wizard's two live checks: GET
// /api/v1/ingress/connectivity (do ports 80 and 443 answer, is the node
// address private) and GET /api/v1/apps/{name}/listening-ports (does the
// running container listen on the configured port).

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DnsProvider } from './domainCheck'

export interface ConnectivityPort {
  port: number
  address: string
  reachable: boolean
}

export type ConnectivityGuidance =
  'ok' | 'no_host' | 'private_address' | 'ports_unreachable'

// Mirrors internal/api's ingressConnectivityResource.
export interface IngressConnectivity {
  host?: string
  host_inferred?: boolean
  addresses: string[]
  private: boolean
  ports: ConnectivityPort[]
  http01_possible: boolean
  dns_provider: DnsProvider
  guidance: ConnectivityGuidance
}

export type ListenVerdict =
  'listening' | 'other_port' | 'not_listening' | 'unknown'

// Mirrors internal/api's listeningPortsResource.
export interface ListeningPorts {
  app: string
  port: number
  probed: boolean
  reason?: 'no_runtime' | 'exec_disabled' | 'not_running' | 'probe_failed'
  listening: number[]
  verdict: ListenVerdict
}

export const domainWizardKeys = {
  connectivity: ['ingress', 'connectivity'] as const,
  listening: (appName: string) => ['apps', appName, 'listening-ports'] as const,
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function ingressConnectivityQueryOptions() {
  return queryOptions({
    queryKey: domainWizardKeys.connectivity,
    queryFn: () =>
      getJson<IngressConnectivity>(
        '/api/v1/ingress/connectivity',
        'check ingress connectivity',
      ),
    staleTime: 15_000,
  })
}

export function useIngressConnectivity(enabled = true) {
  return useQuery({ ...ingressConnectivityQueryOptions(), enabled })
}

export function useListeningPorts(appName: string, enabled = true) {
  return useQuery({
    queryKey: domainWizardKeys.listening(appName),
    queryFn: () =>
      getJson<ListeningPorts>(
        `/api/v1/apps/${encodeURIComponent(appName)}/listening-ports`,
        'check listening ports',
      ),
    enabled,
    staleTime: 10_000,
  })
}
