// GET/PUT /api/v1/settings/ddns and GET /api/v1/settings/ddns/status
// (internal/api/settings_ddns.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type DdnsProvider = 'generic' | 'duckdns'

export const ddnsKeys = {
  settings: ['ddns-settings'] as const,
  status: ['ddns-status'] as const,
}

const STATUS_REFETCH_MS = Number(
  import.meta.env.VITE_DDNS_STATUS_REFETCH_MS ?? 15000,
)

// target is write-only and never part of a read.
export interface DdnsSettings {
  enabled: boolean
  provider: DdnsProvider
  domains?: string
  interval_seconds: number
  probe_urls: string[] | null
  probe_quorum: number
  timeout_seconds: number
  min_update_gap_seconds: number
  reassert_seconds: number
  fail_alert_after: number
  allow_private: boolean
  allow_insecure_http: boolean
  source: 'stored' | 'default'
  target_configured: boolean
  host?: string
  running: boolean
}

// Every field is optional on the wire: omitted means unchanged, and an
// empty-string target clears the stored one.
export type DdnsUpdate = Partial<
  Omit<DdnsSettings, 'source' | 'target_configured' | 'host' | 'running'>
> & { target?: string }

export interface DdnsStatus {
  running: boolean
  last_check?: string
  last_change?: string
  last_ip?: string
  last_result: 'ok' | 'failing' | 'pending' | 'not running'
  last_error?: string
  consecutive_failures: number
  alert_active: boolean
}

async function readJSON<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function ddnsSettingsQueryOptions() {
  return queryOptions({
    queryKey: ddnsKeys.settings,
    queryFn: async () =>
      readJSON<DdnsSettings>(
        await fetch('/api/v1/settings/ddns'),
        'fetch ddns settings',
      ),
    staleTime: 60_000,
    retry: false,
  })
}

export function ddnsStatusQueryOptions() {
  return queryOptions({
    queryKey: ddnsKeys.status,
    queryFn: async () =>
      readJSON<DdnsStatus>(
        await fetch('/api/v1/settings/ddns/status'),
        'fetch ddns status',
      ),
    refetchInterval: STATUS_REFETCH_MS,
    retry: false,
  })
}

export function useDdnsSettings() {
  return useQuery(ddnsSettingsQueryOptions())
}

export function useDdnsStatus() {
  return useQuery(ddnsStatusQueryOptions())
}

export function useUpdateDdnsSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: async (update: DdnsUpdate) =>
      readJSON<DdnsSettings>(
        await fetch('/api/v1/settings/ddns', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(update),
        }),
        'update ddns settings',
      ),
    onSuccess: (updated) => {
      queryClient.setQueryData(ddnsKeys.settings, updated)
      void queryClient.invalidateQueries({ queryKey: ddnsKeys.status })
    },
  })
}
