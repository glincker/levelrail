// /api/v1/logs/usage and /api/v1/apps/{name}/log-budget.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { LogBudget, LogUsage } from '../types/logBudgets'

export const logBudgetKeys = {
  usage: ['log-usage'] as const,
  budget: (app: string) => ['log-budget', app] as const,
}

async function ensureOk(res: Response, fallback: string): Promise<void> {
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
}

export function logUsageQueryOptions() {
  return queryOptions({
    queryKey: logBudgetKeys.usage,
    queryFn: async () => {
      const res = await fetch('/api/v1/logs/usage')
      await ensureOk(res, `load log usage failed: ${res.status}`)
      return (await res.json()) as LogUsage
    },
    refetchInterval: 60_000,
  })
}

export function useLogUsage() {
  return useQuery(logUsageQueryOptions())
}

export function useLogBudget(app: string) {
  return useQuery({
    queryKey: logBudgetKeys.budget(app),
    queryFn: async () => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/log-budget`,
      )
      await ensureOk(res, `load log budget failed: ${res.status}`)
      return (await res.json()) as LogBudget
    },
  })
}

export function useSetLogBudget(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (maxMb: number) => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/log-budget`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ max_mb: maxMb }),
        },
      )
      await ensureOk(res, `save log budget failed: ${res.status}`)
      return (await res.json()) as LogBudget
    },
    onSuccess: (data) => {
      qc.setQueryData(logBudgetKeys.budget(app), data)
      void qc.invalidateQueries({ queryKey: logBudgetKeys.usage })
    },
  })
}
