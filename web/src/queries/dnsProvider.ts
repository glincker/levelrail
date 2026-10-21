// GET/PUT /api/v1/settings/dns-provider and /api/v1/apps/{name}/dns-provider
// (internal/api/dns_provider.go): the free wildcard-DNS provider behind
// automatic app hostnames, plus the resolution test.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DnsProviderInfo {
  id: string
  name: string
  suffix: string
  ip_format: string
  ipv4_only: boolean
  note: string
}

export interface DnsProviderChoice {
  provider: string
  custom_base_domain?: string
  inherited: boolean
}

export interface DnsProviderState {
  providers: DnsProviderInfo[]
  default: DnsProviderChoice
  effective: DnsProviderChoice
  public_host?: string
  hostname?: string
  url?: string
}

export interface DnsProviderRequest {
  provider: string
  custom_base_domain?: string
}

export type DnsTestStatus = 'ok' | 'wrong_ip' | 'not_resolving' | 'unavailable'

export interface DnsProviderFix {
  kind: 'switch_provider' | 'custom_domain'
  provider?: string
}

export interface DnsProviderTestResult {
  status: DnsTestStatus
  provider: string
  hostname?: string
  expected?: string[]
  resolved?: string[]
  message: string
  fixes?: DnsProviderFix[]
  dig_command?: string
}

export const dnsProviderKeys = {
  scope: (app?: string) => ['dns-provider', app ?? ''] as const,
}

function basePath(app?: string): string {
  return app
    ? `/api/v1/apps/${encodeURIComponent(app)}/dns-provider`
    : '/api/v1/settings/dns-provider'
}

async function request<T>(
  path: string,
  method: string,
  body?: DnsProviderRequest,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `dns provider request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function dnsProviderQueryOptions(app?: string) {
  return queryOptions({
    queryKey: dnsProviderKeys.scope(app),
    queryFn: () => request<DnsProviderState>(basePath(app), 'GET'),
  })
}

export function useDnsProvider(app?: string) {
  return useQuery(dnsProviderQueryOptions(app))
}

/** Saving re-derives automatic hostnames, so every dependent view refetches. */
export function useSetDnsProvider(app?: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: DnsProviderRequest) =>
      request<DnsProviderState>(basePath(app), 'PUT', body),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['dns-provider'] }),
        queryClient.invalidateQueries({ queryKey: ['domains'] }),
        queryClient.invalidateQueries({ queryKey: ['apps'] }),
      ])
    },
  })
}

export function useTestDnsProvider(app?: string) {
  return useMutation({
    mutationFn: (body: DnsProviderRequest) =>
      request<DnsProviderTestResult>(`${basePath(app)}/test`, 'POST', body),
  })
}
