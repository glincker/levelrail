import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface BuildHint {
  code: string
  title: string
  explanation: string
  suggestion?: string
  fix?: 'prune_cache'
}

// Matches internal/api's buildRow.
export interface BuildRow {
  id: string
  app: string
  status: string
  source: string
  branch?: string
  commit_sha?: string
  image?: string
  queued_at?: string
  started_at: string
  finished_at?: string
  duration_seconds?: number
  queue_position?: number
  cache_hit_ratio?: number
  image_bytes?: number
  image_delta_bytes?: number
  node?: string
  platform?: string
  error?: string
  hint?: BuildHint
  logs_path: string
  can_cancel: boolean
  can_retry: boolean
}

export interface BuildNodeLoad {
  node: string
  running: number
  waiting: number
  limit: number
}

export interface BuildCenter {
  rows: BuildRow[]
  counts: { queued: number; running: number; finished: number; failed: number }
  nodes: BuildNodeLoad[]
  max_concurrent_per_node: number
  platforms: { host: string; multi_platform: boolean }
}

export interface BuildCacheRecord {
  id: string
  type: string
  description: string
  size_bytes: number
  in_use: boolean
  usage_count: number
  last_used_at?: string
}

export interface BuildCache {
  node: string
  supported: boolean
  reason?: string
  total_bytes: number
  in_use_bytes: number
  reclaimable_bytes: number
  record_count: number
  records: BuildCacheRecord[]
  nodes: { id: string; name: string }[]
}

export interface BuildCachePrune {
  node: string
  applied: boolean
  older_than_hours: number
  candidates: BuildCacheRecord[]
  candidate_count: number
  would_free_bytes: number
  freed_bytes: number
  removed_count: number
}

export const buildKeys = {
  all: ['builds'] as const,
  center: ['builds', 'center'] as const,
  cache: (node: string) => ['builds', 'cache', node] as const,
}

const ACTIVE_REFETCH_MS = 5_000
const IDLE_REFETCH_MS = 30_000

async function json<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function buildCenterQueryOptions() {
  return queryOptions({
    queryKey: buildKeys.center,
    queryFn: async ({ signal }) =>
      json<BuildCenter>(
        await fetch('/api/v1/builds', { signal }),
        'fetch builds',
      ),
    refetchInterval: (q) => {
      const c = q.state.data?.counts
      return c && c.queued + c.running > 0 ? ACTIVE_REFETCH_MS : IDLE_REFETCH_MS
    },
  })
}

export function useBuildCenter() {
  return useQuery(buildCenterQueryOptions())
}

export function useBuildCache(node: string) {
  return useQuery({
    queryKey: buildKeys.cache(node),
    queryFn: async ({ signal }) =>
      json<BuildCache>(
        await fetch(`/api/v1/builds/cache?node=${encodeURIComponent(node)}`, {
          signal,
        }),
        'fetch build cache',
      ),
  })
}

export function useBuildAction(action: 'cancel' | 'retry') {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (row: Pick<BuildRow, 'app' | 'id'>): Promise<void> => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(row.app)}/builds/${encodeURIComponent(row.id)}/${action}`,
        { method: 'POST' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `${action} build failed: ${res.status}`),
        )
      }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: buildKeys.center }),
  })
}

export function usePruneBuildCache(node: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (apply: boolean): Promise<BuildCachePrune> =>
      json<BuildCachePrune>(
        await fetch('/api/v1/builds/cache/prune', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ node, apply }),
        }),
        'prune build cache',
      ),
    onSuccess: (res) => {
      if (res.applied) void qc.invalidateQueries({ queryKey: ['builds'] })
    },
  })
}
