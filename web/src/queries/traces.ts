// /api/v1/apps/{name}/traces and /tracing (internal/api/traces.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { TraceSpan, TraceSummary, Tracing } from '../types/traces'

export interface TraceFilters {
  errorsOnly: boolean
  minMs: number
  windowMinutes: number
}

export const traceKeys = {
  all: (app: string) => ['traces', app] as const,
  list: (app: string, f: TraceFilters) =>
    [...traceKeys.all(app), 'list', f] as const,
  detail: (app: string, id: string) =>
    [...traceKeys.all(app), 'detail', id] as const,
  tracing: (app: string) => [...traceKeys.all(app), 'tracing'] as const,
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

export function traceListQueryOptions(app: string, f: TraceFilters) {
  return queryOptions({
    queryKey: traceKeys.list(app, f),
    refetchInterval: 15_000,
    queryFn: async () => {
      const q = new URLSearchParams()
      q.set(
        'from',
        new Date(Date.now() - f.windowMinutes * 60_000).toISOString(),
      )
      if (f.errorsOnly) q.set('errors', 'true')
      if (f.minMs > 0) q.set('min_ms', String(f.minMs))
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/traces?${q.toString()}`,
      )
      return json<TraceSummary[]>(res, 'list traces')
    },
  })
}

export function useTraces(app: string, f: TraceFilters) {
  return useQuery(traceListQueryOptions(app, f))
}

export function useTrace(app: string, id: string | null) {
  return useQuery({
    queryKey: traceKeys.detail(app, id ?? ''),
    enabled: id !== null,
    queryFn: async () => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/traces/${encodeURIComponent(id ?? '')}`,
      )
      return json<TraceSpan[]>(res, 'get trace')
    },
  })
}

export function useTracing(app: string) {
  return useQuery({
    queryKey: traceKeys.tracing(app),
    queryFn: async () => {
      const res = await fetch(`/api/v1/apps/${encodeURIComponent(app)}/tracing`)
      return json<Tracing>(res, 'get tracing')
    },
  })
}

export function useEnableTracing(app: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/tracing`,
        { method: 'POST' },
      )
      return json<Tracing>(res, 'enable tracing')
    },
    onSuccess: (data) => {
      client.setQueryData(traceKeys.tracing(app), {
        ...data,
        key: undefined,
        env: undefined,
      })
    },
  })
}

export function useDisableTracing(app: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/tracing`,
        { method: 'DELETE' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `disable tracing failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: traceKeys.tracing(app) })
    },
  })
}
