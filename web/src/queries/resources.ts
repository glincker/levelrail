import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { usageKeys } from './usage'

export type ResourceKind = 'app' | 'database' | 'node' | 'domain'

export const RESOURCE_KINDS: ResourceKind[] = [
  'app',
  'database',
  'node',
  'domain',
]

// Matches internal/api's consoleResource.
export interface ConsoleResource {
  id?: string
  kind: ResourceKind
  name: string
  detail?: string
  status?: string
  tags: string[]
}

export interface ConsoleList {
  items: ConsoleResource[]
  total: number
  counts: Partial<Record<ResourceKind, number>>
}

export interface ConsoleQuery {
  q: string
  kind: ResourceKind | ''
  tags: string[]
}

export type BulkTagAction = 'add' | 'remove'

export interface BulkTagRequest {
  action: BulkTagAction
  tag: string
  resources: { kind: ResourceKind; name: string }[]
  dry_run: boolean
}

export interface BulkTagResult {
  kind: ResourceKind
  name: string
  status: 'ok' | 'would_apply' | 'skipped' | 'denied' | 'not_found' | 'error'
  message?: string
}

export interface BulkTagResponse {
  action: BulkTagAction
  tag: string
  dry_run: boolean
  results: BulkTagResult[]
  counts: Record<string, number>
}

export interface ConsoleView {
  id: string
  name: string
  query: string
  created_at: string
}

export const resourceKeys = {
  all: ['resources'] as const,
  list: (q: ConsoleQuery) => [...resourceKeys.all, 'list', q] as const,
  views: () => [...resourceKeys.all, 'views'] as const,
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  // The bulk endpoint answers 207 with per-item results, which is still ok.
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function listUrl(q: ConsoleQuery): string {
  const p = new URLSearchParams()
  if (q.q.trim()) p.set('q', q.q.trim())
  if (q.kind) p.set('kind', q.kind)
  for (const t of q.tags) p.append('tag', t)
  const s = p.toString()
  return s ? `/api/v1/resources?${s}` : '/api/v1/resources'
}

export function resourceListQueryOptions(q: ConsoleQuery) {
  return queryOptions({
    queryKey: resourceKeys.list(q),
    queryFn: ({ signal }) =>
      request<ConsoleList>(listUrl(q), { signal }, 'list resources'),
    staleTime: 15_000,
  })
}

export function useResourceList(q: ConsoleQuery) {
  return useQuery(resourceListQueryOptions(q))
}

export function useBulkTagResources() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: BulkTagRequest) =>
      request<BulkTagResponse>(
        '/api/v1/resources/tags/bulk',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
        'tag resources',
      ),
    onSuccess: (_data, req) => {
      if (req.dry_run) return
      void qc.invalidateQueries({ queryKey: resourceKeys.all })
      void qc.invalidateQueries({ queryKey: ['tags'] })
      void qc.invalidateQueries({ queryKey: ['apps'] })
      void qc.invalidateQueries({ queryKey: usageKeys.all })
    },
  })
}

export function useConsoleViews() {
  return useQuery({
    queryKey: resourceKeys.views(),
    queryFn: ({ signal }) =>
      request<ConsoleView[]>(
        '/api/v1/resources/views',
        { signal },
        'list saved views',
      ),
    staleTime: 30_000,
  })
}

export function useSaveConsoleView() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { name: string; query: string }) =>
      request<ConsoleView>(
        '/api/v1/resources/views',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(v),
        },
        'save view',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: resourceKeys.views() }),
  })
}

export function useDeleteConsoleView() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `/api/v1/resources/views/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'delete view',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: resourceKeys.views() }),
  })
}
