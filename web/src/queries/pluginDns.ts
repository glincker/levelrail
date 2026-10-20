import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { goLiveKeys } from './goLive'

// Matches internal/api's dnsProviderPluginResource.
export interface DnsProviderPlugin {
  plugin: string
  provider: string
}

// Matches internal/api's dnsPluginTestResult.
export interface PluginTestResult {
  ok: boolean
  zones?: string[]
  message: string
}

const SETTING_URL = '/api/v1/settings/dns-provider-plugin'
const settingKey = ['settings', 'dns-provider-plugin'] as const

async function call<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    cache: 'no-store',
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useDnsProviderPlugin() {
  return useQuery({
    queryKey: settingKey,
    queryFn: () => call<DnsProviderPlugin>('GET', SETTING_URL),
    staleTime: 15_000,
  })
}

export function useSetDnsProviderPlugin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (plugin: string) =>
      call<DnsProviderPlugin>('PUT', SETTING_URL, { plugin }),
    onSuccess: (data) => {
      qc.setQueryData(settingKey, data)
      void qc.invalidateQueries({ queryKey: goLiveKeys.automation })
    },
  })
}

export function useTestDnsPlugin() {
  return useMutation({
    mutationFn: (name: string) =>
      call<PluginTestResult>(
        'POST',
        `/api/v1/plugins/${encodeURIComponent(name)}/test`,
      ),
  })
}
