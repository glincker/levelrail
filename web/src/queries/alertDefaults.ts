// Default alerts seeded on first boot (internal/api/alert_defaults.go).

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { useApiMutation } from './useApiMutation'

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
  return useApiMutation({
    fn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      request<DefaultAlert>(
        `/api/v1/alert-defaults/${encodeURIComponent(id)}`,
        'Updating default alert',
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled }),
        },
      ),
    invalidate: [defaultAlertKeys.all],
  })
}

export function useResetDefaultAlerts() {
  return useApiMutation({
    fn: () =>
      request<DefaultAlert[]>(
        '/api/v1/alert-defaults/reset',
        'Resetting default alerts',
        { method: 'POST' },
      ),
    invalidate: [defaultAlertKeys.all],
  })
}

export interface DefaultAlertSettings {
  enabled: boolean
  crashloop_restarts: number
  error_rate_percent: number
  digest_schedule: string
}

export const DIGEST_ON_SCHEDULE = 'mon@08'

const settingsKey = [...defaultAlertKeys.all, 'settings'] as const

export function useDefaultAlertSettings() {
  return useQuery({
    queryKey: settingsKey,
    queryFn: () =>
      request<DefaultAlertSettings>(
        '/api/v1/alert-defaults/settings',
        'Loading default alert settings',
      ),
  })
}

export function useUpdateDefaultAlertSettings() {
  return useApiMutation({
    fn: (patch: Partial<DefaultAlertSettings>) =>
      request<DefaultAlertSettings>(
        '/api/v1/alert-defaults/settings',
        'Updating default alert settings',
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(patch),
        },
      ),
    invalidate: [defaultAlertKeys.all],
  })
}
