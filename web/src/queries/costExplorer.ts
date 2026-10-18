// Queries and mutations for /api/v1/costs: the cost explorer and named budgets.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  CostBudget,
  CostBudgetInput,
  CostBudgets,
  CostExploration,
  CostExploreParams,
} from '../types/costExplorer'

export const costKeys = {
  all: ['costs'] as const,
  explore: (p: CostExploreParams) => [...costKeys.all, 'explore', p] as const,
  budgets: () => [...costKeys.all, 'budgets'] as const,
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

function send(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function costExploreQueryOptions(p: CostExploreParams) {
  const q = new URLSearchParams({
    group_by: p.groupBy,
    granularity: p.granularity,
  })
  if (p.granularity === 'monthly') q.set('months', String(p.months))
  else q.set('period', p.period)
  return queryOptions({
    queryKey: costKeys.explore(p),
    queryFn: () =>
      request<CostExploration>(
        `/api/v1/costs/explore?${q.toString()}`,
        {},
        'fetch cost explorer',
      ),
    staleTime: 60_000,
  })
}

export function useCostExplore(p: CostExploreParams) {
  return useQuery(costExploreQueryOptions(p))
}

// A 403 means the caller is not an instance admin: the page hides budgets.
export function useCostBudgets() {
  return useQuery({
    queryKey: costKeys.budgets(),
    queryFn: () =>
      request<CostBudgets>('/api/v1/costs/budgets', {}, 'fetch cost budgets'),
    staleTime: 30_000,
    retry: false,
  })
}

export function useCreateCostBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CostBudgetInput) =>
      request<CostBudget>(
        '/api/v1/costs/budgets',
        send('POST', body),
        'create budget',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: costKeys.budgets() }),
  })
}

export function useUpdateCostBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: CostBudgetInput }) =>
      request<CostBudget>(
        `/api/v1/costs/budgets/${encodeURIComponent(id)}`,
        send('PUT', body),
        'update budget',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: costKeys.budgets() }),
  })
}

export function useDeleteCostBudget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `/api/v1/costs/budgets/${encodeURIComponent(id)}`,
        send('DELETE'),
        'delete budget',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: costKeys.budgets() }),
  })
}
