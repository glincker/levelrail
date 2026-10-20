// /api/v1/apps/{name}/errors and /error-tracking (internal/api/errors_issues.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  ErrorIssue,
  ErrorIssueDetail,
  ErrorStatus,
  ErrorTracking,
} from '../types/errorTracking'

export interface ErrorFilters {
  status: ErrorStatus | 'all'
  sinceHours: number
}

export const errorKeys = {
  all: (app: string) => ['errors', app] as const,
  list: (app: string, f: ErrorFilters) =>
    [...errorKeys.all(app), 'list', f] as const,
  detail: (app: string, fp: string) =>
    [...errorKeys.all(app), 'detail', fp] as const,
  setup: (app: string) => [...errorKeys.all(app), 'setup'] as const,
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

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}`
}

export function errorListQueryOptions(app: string, f: ErrorFilters) {
  return queryOptions({
    queryKey: errorKeys.list(app, f),
    refetchInterval: 15_000,
    queryFn: async () => {
      const q = new URLSearchParams()
      q.set('since', `${f.sinceHours}h`)
      if (f.status !== 'all') q.set('status', f.status)
      const res = await fetch(`${base(app)}/errors?${q.toString()}`)
      return json<ErrorIssue[]>(res, 'list errors')
    },
  })
}

export function useErrorIssues(app: string, f: ErrorFilters) {
  return useQuery(errorListQueryOptions(app, f))
}

export function useErrorIssue(app: string, fingerprint: string | null) {
  return useQuery({
    queryKey: errorKeys.detail(app, fingerprint ?? ''),
    enabled: fingerprint !== null,
    queryFn: async () => {
      const res = await fetch(
        `${base(app)}/errors/${encodeURIComponent(fingerprint ?? '')}`,
      )
      return json<ErrorIssueDetail>(res, 'get error')
    },
  })
}

export function useSetErrorStatus(app: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (v: { fingerprint: string; status: ErrorStatus }) => {
      const res = await fetch(
        `${base(app)}/errors/${encodeURIComponent(v.fingerprint)}`,
        {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ status: v.status }),
        },
      )
      return json<ErrorIssueDetail>(res, 'update error')
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: errorKeys.all(app) })
    },
  })
}

export function useErrorTracking(app: string) {
  return useQuery({
    queryKey: errorKeys.setup(app),
    queryFn: async () => {
      const res = await fetch(`${base(app)}/error-tracking`)
      return json<ErrorTracking>(res, 'get error tracking')
    },
  })
}

export function useEnableErrorTracking(app: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const res = await fetch(`${base(app)}/error-tracking`, {
        method: 'POST',
      })
      return json<ErrorTracking>(res, 'enable error tracking')
    },
    onSuccess: (data) => {
      client.setQueryData(errorKeys.setup(app), {
        ...data,
        dsn: undefined,
        env: undefined,
      })
    },
  })
}

export function useDisableErrorTracking(app: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const res = await fetch(`${base(app)}/error-tracking`, {
        method: 'DELETE',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `disable error tracking failed: ${res.status}`,
          ),
        )
      }
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: errorKeys.setup(app) })
    },
  })
}
