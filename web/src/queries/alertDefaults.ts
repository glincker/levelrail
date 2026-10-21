// Default alerts seeded on first boot (internal/api/alert_defaults.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DefaultAlert {
  id: string
  name: string
  kind: string
  severity: string
  enabled: boolean
  firing: boolean
  lead_time?: string
}

export const defaultAlertKeys = {
  all: ['alert-defaults'] as const,
}

async function request<T>(url: string, what: string, init?: RequestInit) {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function defaultAlertsQueryOptions() {
  return queryOptions({
    queryKey: defaultAlertKeys.all,
    queryFn: () =>
      request<DefaultAlert[]>(
        '/api/v1/alert-defaults',
        'Loading default alerts',
      ),
  })
}

export function useDefaultAlerts() {
  return useQuery(defaultAlertsQueryOptions())
}

export function useSetDefaultAlertEnabled() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      request<DefaultAlert>(
        `/api/v1/alert-defaults/${encodeURIComponent(id)}`,
        'Updating default alert',
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled }),
        },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: defaultAlertKeys.all }),
  })
}

export function useResetDefaultAlerts() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      request<DefaultAlert[]>(
        '/api/v1/alert-defaults/reset',
        'Resetting default alerts',
        { method: 'POST' },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: defaultAlertKeys.all }),
  })
}
