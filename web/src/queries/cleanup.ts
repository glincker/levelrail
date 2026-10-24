// Fetchers for /api/v1/cleanup (internal/cleanup). Retention days are saved through runtime settings.

import {
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { runsQuery } from '../lib/cleanup'
import type {
  CleanupImpact,
  CleanupMode,
  CleanupOverview,
  CleanupResult,
  CleanupRunFilters,
  CleanupRunsPage,
} from '../types/cleanup'

const BASE = '/api/v1/cleanup'

export const RUNS_PAGE_SIZE = 25

export const cleanupKeys = {
  all: ['cleanup'] as const,
  overview: () => [...cleanupKeys.all, 'overview'] as const,
  runs: (filters: CleanupRunFilters) =>
    [...cleanupKeys.all, 'runs', filters] as const,
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

function send(body: unknown, method = 'POST'): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function cleanupOverviewQueryOptions() {
  return queryOptions({
    queryKey: cleanupKeys.overview(),
    queryFn: async () =>
      json<CleanupOverview>(await fetch(BASE), 'load cleanup targets'),
    staleTime: 10_000,
  })
}

export function useCleanupOverview() {
  return useQuery(cleanupOverviewQueryOptions())
}

export function usePreviewCleanup() {
  return useMutation({
    mutationFn: async (v: { id: string; days: number }) =>
      json<CleanupImpact>(
        await fetch(
          `${BASE}/${encodeURIComponent(v.id)}/preview`,
          send({ days: v.days }),
        ),
        'preview cleanup',
      ),
  })
}

export function useRunCleanup() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (v: { id: string; days: number }) =>
      json<CleanupResult>(
        await fetch(
          `${BASE}/${encodeURIComponent(v.id)}/run`,
          send({ days: v.days, dry_run: false }),
        ),
        'run cleanup',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: cleanupKeys.all }),
  })
}

export function useSetCleanupMode() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (v: { id: string; mode: CleanupMode }) => {
      const res = await fetch(
        `${BASE}/${encodeURIComponent(v.id)}/mode`,
        send({ mode: v.mode }, 'PUT'),
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `change mode failed: ${res.status}`),
        )
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: cleanupKeys.all }),
  })
}

export function useCleanupRuns(filters: CleanupRunFilters) {
  return useInfiniteQuery({
    queryKey: cleanupKeys.runs(filters),
    initialPageParam: '',
    queryFn: async ({ pageParam }): Promise<CleanupRunsPage> => {
      const qs = runsQuery(filters, {
        limit: String(RUNS_PAGE_SIZE),
        ...(pageParam ? { cursor: pageParam } : {}),
      })
      return json<CleanupRunsPage>(
        await fetch(`${BASE}/runs?${qs}`),
        'load cleanup history',
      )
    },
    getNextPageParam: (last) => last.next_cursor || undefined,
  })
}
