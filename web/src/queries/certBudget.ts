// Certificate issuance budget and the per-app automatic address switch.
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface CertBudget {
  issued: number
  warn: number
  window_days: number
  over: boolean
  checked_at: string
}

export interface AutoHost {
  enabled: boolean
  hostname?: string
}

export const certBudgetKeys = {
  all: ['cert-budget'] as const,
  budget: () => [...certBudgetKeys.all, 'budget'] as const,
  autoHost: (app: string) => [...certBudgetKeys.all, 'auto-host', app] as const,
}

async function json<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function certBudgetQueryOptions() {
  return queryOptions({
    queryKey: certBudgetKeys.budget(),
    queryFn: async () =>
      json<CertBudget>(
        await fetch('/api/v1/certificates/budget'),
        'fetch certificate budget',
      ),
    staleTime: 15_000,
  })
}

export function autoHostQueryOptions(app: string) {
  return queryOptions({
    queryKey: certBudgetKeys.autoHost(app),
    queryFn: async () =>
      json<AutoHost>(
        await fetch(`/api/v1/apps/${encodeURIComponent(app)}/auto-host`),
        'fetch automatic address',
      ),
    staleTime: 15_000,
  })
}

export function useSetAutoHost() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (v: { app: string; enabled: boolean }) =>
      json<AutoHost>(
        await fetch(`/api/v1/apps/${encodeURIComponent(v.app)}/auto-host`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled: v.enabled }),
        }),
        'save automatic address',
      ),
    onSuccess: (_data, v) => {
      void queryClient.invalidateQueries({
        queryKey: certBudgetKeys.autoHost(v.app),
      })
      void queryClient.invalidateQueries({ queryKey: certBudgetKeys.budget() })
    },
  })
}
