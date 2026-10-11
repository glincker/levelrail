// Queries and mutations for /api/v1/usage: cost showback, prices and budgets.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  UsageBudget,
  UsageBudgets,
  UsagePeriod,
  UsageReport,
  UsageSettings,
  UsageSettingsUpdate,
} from '../types/usage'

export const usageKeys = {
  all: ['usage'] as const,
  report: (period: UsagePeriod) =>
    [...usageKeys.all, 'report', period] as const,
  settings: () => [...usageKeys.all, 'settings'] as const,
  budgets: () => [...usageKeys.all, 'budgets'] as const,
}

async function request<T>(
  url: string,
  init: RequestInit,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function send(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function usageReportQueryOptions(period: UsagePeriod) {
  return queryOptions({
    queryKey: usageKeys.report(period),
    queryFn: () =>
      request<UsageReport>(
        `/api/v1/usage/report?period=${period}`,
        {},
        'fetch usage report',
      ),
    staleTime: 60_000,
  })
}

export function useUsageReport(period: UsagePeriod, enabled = true) {
  return useQuery({ ...usageReportQueryOptions(period), enabled })
}

// A 403 means the caller is not an instance admin: the page hides prices
// rather than showing an error.
export function useUsageSettings() {
  return useQuery({
    queryKey: usageKeys.settings(),
    queryFn: () =>
      request<UsageSettings>(
        '/api/v1/usage/settings',
        {},
        'fetch usage prices',
      ),
    retry: false,
    staleTime: 60_000,
  })
}

export function useSaveUsageSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (update: UsageSettingsUpdate) =>
      request<UsageSettings>(
        '/api/v1/usage/settings',
        send('PUT', update),
        'save usage prices',
      ),
    onSuccess: (data) => {
      qc.setQueryData(usageKeys.settings(), data)
      void qc.invalidateQueries({ queryKey: [...usageKeys.all, 'report'] })
      void qc.invalidateQueries({ queryKey: usageKeys.budgets() })
    },
  })
}

export function useUsageBudgets() {
  return useQuery({
    queryKey: usageKeys.budgets(),
    queryFn: () =>
      request<UsageBudgets>('/api/v1/usage/budgets', {}, 'fetch budgets'),
    staleTime: 60_000,
  })
}

export function useSaveUsageBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (b: {
      scope: 'project' | 'team'
      scope_id: string
      monthly: number
      warn_pct: number
    }) =>
      request<UsageBudget>(
        '/api/v1/usage/budgets',
        send('PUT', b),
        'save budget',
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: usageKeys.budgets() })
    },
  })
}

export function useDeleteUsageBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `/api/v1/usage/budgets/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'delete budget',
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: usageKeys.budgets() })
    },
  })
}
