// Query keys, fetchers and hooks for silences, maintenance windows and
// alert history (internal/api/alert_noise.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { alertRuleKeys } from './alerts'
import type {
  AlertHistoryEntry,
  AlertHistoryFilter,
  CreateSilenceRequest,
  MaintenanceWindow,
  Silence,
} from '../types/alertNoise'

export const alertNoiseKeys = {
  all: ['alert-noise'] as const,
  silences: (includeExpired: boolean) =>
    [...alertNoiseKeys.all, 'silences', includeExpired] as const,
  windows: () => [...alertNoiseKeys.all, 'windows'] as const,
  history: (filter: AlertHistoryFilter) =>
    [...alertNoiseKeys.all, 'history', filter] as const,
}

async function request<T>(
  url: string,
  what: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(url, init)
  if (res.status === 501) {
    throw new ApiError(501, 'alerting is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function silencesQueryOptions(includeExpired: boolean) {
  return queryOptions({
    queryKey: alertNoiseKeys.silences(includeExpired),
    queryFn: () =>
      request<Silence[]>(
        `/api/v1/alert-silences${includeExpired ? '?include_expired=true' : ''}`,
        'list silences',
      ),
  })
}

export function useSilences(includeExpired: boolean) {
  return useQuery(silencesQueryOptions(includeExpired))
}

export function useCreateSilence() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateSilenceRequest) =>
      request<Silence>(
        '/api/v1/alert-silences',
        'create silence',
        jsonInit('POST', req),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: alertNoiseKeys.all })
    },
  })
}

export function useExpireSilence() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<Silence>(
        `/api/v1/alert-silences/${encodeURIComponent(id)}`,
        'expire silence',
        { method: 'DELETE' },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: alertNoiseKeys.all })
    },
  })
}

// POST /api/v1/apps/{name}/alerts/{id}/silence: the quick action.
export function useSilenceRule(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ruleId, duration }: { ruleId: string; duration: string }) =>
      request<Silence>(
        `/api/v1/apps/${encodeURIComponent(appName)}/alerts/${encodeURIComponent(ruleId)}/silence`,
        'silence alert rule',
        jsonInit('POST', { duration }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: alertNoiseKeys.all })
      void queryClient.invalidateQueries({
        queryKey: alertRuleKeys.all(appName),
      })
    },
  })
}

export function maintenanceWindowsQueryOptions() {
  return queryOptions({
    queryKey: alertNoiseKeys.windows(),
    queryFn: () =>
      request<MaintenanceWindow[]>(
        '/api/v1/alert-maintenance-windows',
        'list maintenance windows',
      ),
  })
}

export function useMaintenanceWindows() {
  return useQuery(maintenanceWindowsQueryOptions())
}

export function useSaveMaintenanceWindow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (w: MaintenanceWindow) =>
      request<MaintenanceWindow>(
        w.id
          ? `/api/v1/alert-maintenance-windows/${encodeURIComponent(w.id)}`
          : '/api/v1/alert-maintenance-windows',
        'save maintenance window',
        jsonInit(w.id ? 'PUT' : 'POST', w),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: alertNoiseKeys.windows() })
    },
  })
}

export function useDeleteMaintenanceWindow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `/api/v1/alert-maintenance-windows/${encodeURIComponent(id)}`,
        'delete maintenance window',
        { method: 'DELETE' },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: alertNoiseKeys.windows() })
    },
  })
}

export function useAlertHistory(filter: AlertHistoryFilter) {
  const params = new URLSearchParams()
  if (filter.outcome) params.set('outcome', filter.outcome)
  if (filter.event) params.set('event', filter.event)
  params.set('limit', String(filter.limit ?? 50))
  const path = filter.app
    ? `/api/v1/apps/${encodeURIComponent(filter.app)}/alert-history`
    : '/api/v1/alert-history'
  return useQuery({
    queryKey: alertNoiseKeys.history(filter),
    queryFn: () =>
      request<AlertHistoryEntry[]>(
        `${path}?${params.toString()}`,
        'list alert history',
      ),
    refetchInterval: 30_000,
  })
}
