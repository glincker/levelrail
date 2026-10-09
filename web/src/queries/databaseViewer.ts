// Fetchers and hooks for the database viewer routes
// (internal/api/database_viewer*.go). Row data is sensitive, so none of
// these are cached beyond the session's own query cache lifetime.

import {
  keepPreviousData,
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { databaseKeys } from './databases'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  DbPage,
  DbPageParams,
  DbQueryHistoryEntry,
  DbQueryResponse,
  DbSavedQuery,
  DbSchemaResponse,
  RedisScan,
  RedisValue,
} from '../types/databaseViewer'

const base = (name: string) => `/api/v1/databases/${encodeURIComponent(name)}`

export const databaseViewerKeys = {
  all: (name: string) => [...databaseKeys.detail(name), 'viewer'] as const,
  schema: (name: string) =>
    [...databaseViewerKeys.all(name), 'schema'] as const,
  page: (name: string, p: DbPageParams) =>
    [...databaseViewerKeys.all(name), 'page', p] as const,
  keys: (name: string, pattern: string) =>
    [...databaseViewerKeys.all(name), 'keys', pattern] as const,
  key: (name: string, key: string) =>
    [...databaseViewerKeys.all(name), 'key', key] as const,
  history: (name: string) =>
    [...databaseViewerKeys.all(name), 'history'] as const,
  saved: (name: string) => [...databaseViewerKeys.all(name), 'saved'] as const,
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  fallback: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

function postJson(body: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function useDatabaseSchema(name: string, enabled: boolean) {
  return useQuery(
    queryOptions({
      queryKey: databaseViewerKeys.schema(name),
      queryFn: () =>
        request<DbSchemaResponse>(`${base(name)}/schema`, undefined, 'schema'),
      enabled,
      staleTime: 0,
    }),
  )
}

export function useDatabaseTablePage(name: string, p: DbPageParams) {
  return useQuery({
    queryKey: databaseViewerKeys.page(name, p),
    queryFn: () => {
      const q = new URLSearchParams({
        limit: String(p.limit),
        offset: String(p.offset),
      })
      if (p.sort) {
        q.set('sort', p.sort)
        q.set('dir', p.desc ? 'desc' : 'asc')
      }
      if (p.filterColumn && p.filterOp) {
        q.set('filter_column', p.filterColumn)
        q.set('filter_op', p.filterOp)
        if (p.filterValue) {
          q.set('filter_value', p.filterValue)
        }
      }
      const path = `${base(name)}/tables/${encodeURIComponent(p.schema)}/${encodeURIComponent(p.table)}/rows?${q.toString()}`
      return request<DbPage>(path, undefined, 'table rows')
    },
    placeholderData: keepPreviousData,
    staleTime: 0,
  })
}

export interface RunQueryInput {
  name: string
  sql: string
  mode: 'read' | 'write' | 'explain'
  analyze?: boolean
  confirm?: string
}

export function useRunDatabaseQuery() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, sql, mode, analyze, confirm }: RunQueryInput) => {
      const path =
        mode === 'write'
          ? `${base(name)}/query/write`
          : mode === 'explain'
            ? `${base(name)}/explain`
            : `${base(name)}/query`
      return request<DbQueryResponse>(
        path,
        postJson({ sql, analyze, confirm }),
        'query failed',
      )
    },
    onSettled: (_data, _err, { name }) => {
      void queryClient.invalidateQueries({
        queryKey: databaseViewerKeys.history(name),
      })
    },
  })
}

export function useQueryHistory(name: string) {
  return useQuery({
    queryKey: databaseViewerKeys.history(name),
    queryFn: () =>
      request<DbQueryHistoryEntry[]>(
        `${base(name)}/query-history`,
        undefined,
        'query history',
      ),
  })
}

export function useClearQueryHistory(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      request<void>(
        `${base(name)}/query-history`,
        { method: 'DELETE' },
        'clear history',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: databaseViewerKeys.history(name),
      })
    },
  })
}

export function useSavedQueries(name: string) {
  return useQuery({
    queryKey: databaseViewerKeys.saved(name),
    queryFn: () =>
      request<DbSavedQuery[]>(
        `${base(name)}/saved-queries`,
        undefined,
        'saved queries',
      ),
  })
}

export function useSaveQuery(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string; sql: string }) =>
      request<DbSavedQuery>(
        `${base(name)}/saved-queries`,
        postJson(input),
        'save query',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: databaseViewerKeys.saved(name),
      })
    },
  })
}

export function useDeleteSavedQuery(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `${base(name)}/saved-queries/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'delete saved query',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: databaseViewerKeys.saved(name),
      })
    },
  })
}

export const SCAN_DONE_CURSOR = '0'

export function useRedisKeys(name: string, pattern: string) {
  return useInfiniteQuery({
    queryKey: databaseViewerKeys.keys(name, pattern),
    initialPageParam: SCAN_DONE_CURSOR,
    queryFn: ({ pageParam }) => {
      const q = new URLSearchParams({ cursor: pageParam, pattern })
      return request<RedisScan>(
        `${base(name)}/keys?${q.toString()}`,
        undefined,
        'keys',
      )
    },
    getNextPageParam: (last) =>
      last.cursor === SCAN_DONE_CURSOR ? undefined : last.cursor,
    staleTime: 0,
  })
}

export function useRedisKey(name: string, key: string | null) {
  return useQuery({
    queryKey: databaseViewerKeys.key(name, key ?? ''),
    queryFn: () =>
      request<RedisValue>(
        `${base(name)}/key?key=${encodeURIComponent(key ?? '')}`,
        undefined,
        'key',
      ),
    enabled: key !== null,
    staleTime: 0,
  })
}
