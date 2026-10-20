// Query hooks for the log explorer: error signatures
// (GET /api/v1/apps/{name}/logs/signatures) and saved queries
// (/api/v1/logs/queries).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  LogSignatures,
  SavedLogQuery,
  SavedLogQueryRequest,
} from '../types/logExplorer'
import { logSearchKeys } from './logs'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const logExplorerKeys = {
  signatures: (
    appName: string,
    fromIso: string,
    toIso: string,
    level: string,
    baseline: boolean,
  ) =>
    [
      ...logSearchKeys.all(appName),
      'signatures',
      fromIso,
      toIso,
      level,
      baseline,
    ] as const,
  saved: (appName: string) => ['log-queries', appName] as const,
}

export interface SignatureParams {
  from: Date
  to: Date
  level?: string
  baseline?: boolean
  limit?: number
}

export async function fetchLogSignatures(
  appName: string,
  p: SignatureParams,
): Promise<LogSignatures> {
  const q = new URLSearchParams({
    from: p.from.toISOString(),
    to: p.to.toISOString(),
  })
  if (p.level) {
    q.set('level', p.level)
  }
  if (p.baseline) {
    q.set('baseline', 'true')
  }
  if (p.limit) {
    q.set('limit', String(p.limit))
  }
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/logs/signatures?${q.toString()}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch log signatures failed: ${res.status}`),
    )
  }
  return (await res.json()) as LogSignatures
}

export function logSignaturesQueryOptions(appName: string, p: SignatureParams) {
  return queryOptions({
    queryKey: logExplorerKeys.signatures(
      appName,
      p.from.toISOString(),
      p.to.toISOString(),
      p.level ?? '',
      p.baseline ?? false,
    ),
    queryFn: () => fetchLogSignatures(appName, p),
  })
}

export function useLogSignatures(appName: string, p: SignatureParams) {
  return useQuery(logSignaturesQueryOptions(appName, p))
}

async function savedQueryError(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function useSavedLogQueries(appName: string) {
  return useQuery({
    queryKey: logExplorerKeys.saved(appName),
    queryFn: async (): Promise<SavedLogQuery[]> => {
      const res = await fetch(
        `/api/v1/logs/queries?app=${encodeURIComponent(appName)}`,
      )
      if (!res.ok) {
        throw await savedQueryError(res, 'list saved log queries')
      }
      const body = (await res.json()) as { queries?: SavedLogQuery[] }
      return body.queries ?? []
    },
  })
}

export function useSaveLogQuery(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: SavedLogQueryRequest): Promise<SavedLogQuery> => {
      const res = await fetch('/api/v1/logs/queries', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
      if (!res.ok) {
        throw await savedQueryError(res, 'save log query')
      }
      return (await res.json()) as SavedLogQuery
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: logExplorerKeys.saved(appName),
      })
    },
  })
}

export function useDeleteLogQuery(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string): Promise<void> => {
      const res = await fetch(
        `/api/v1/logs/queries/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
      )
      if (!res.ok) {
        throw await savedQueryError(res, 'delete log query')
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: logExplorerKeys.saved(appName),
      })
    },
  })
}
