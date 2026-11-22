// GET/PUT/DELETE and POST .../test on /api/v1/settings/dns-providers/{provider}
// (internal/api/dns_token_providers.go). The credential is write-only.

import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { useApiMutation } from './useApiMutation'

export type DnsTokenProviderName = 'godaddy' | 'namecheap'

export interface DnsTokenProviderSettings {
  provider: string
  enabled: boolean
  has_key: boolean
}

export interface DnsTokenTestResult {
  ok: boolean
  zones?: string[]
  error?: string
}

export const dnsTokenProviderKeys = {
  one: (name: DnsTokenProviderName) => ['dns-token-provider', name] as const,
}

const path = (name: DnsTokenProviderName) =>
  `/api/v1/settings/dns-providers/${name}`

async function send<T>(
  url: string,
  method: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${url} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function dnsTokenProviderQueryOptions(name: DnsTokenProviderName) {
  return queryOptions({
    queryKey: dnsTokenProviderKeys.one(name),
    queryFn: () => send<DnsTokenProviderSettings>(path(name), 'GET'),
    staleTime: 60_000,
  })
}

export function useUpdateDnsTokenProvider(name: DnsTokenProviderName) {
  return useApiMutation<
    DnsTokenProviderSettings,
    { enabled: boolean; credential?: string }
  >({
    fn: (req) => send<DnsTokenProviderSettings>(path(name), 'PUT', req),
    invalidate: [dnsTokenProviderKeys.one(name), ['dns', 'zones']],
  })
}

export function useDisconnectDnsTokenProvider(name: DnsTokenProviderName) {
  return useApiMutation<DnsTokenProviderSettings>({
    fn: () => send<DnsTokenProviderSettings>(path(name), 'DELETE'),
    invalidate: [dnsTokenProviderKeys.one(name), ['dns', 'zones']],
  })
}

export function useTestDnsTokenProvider(name: DnsTokenProviderName) {
  return useApiMutation<DnsTokenTestResult>({
    fn: () => send<DnsTokenTestResult>(`${path(name)}/test`, 'POST'),
    invalidate: [],
  })
}
