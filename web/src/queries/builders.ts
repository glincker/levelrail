// Builder read model: GET /api/v1/builders, /builders/{id}, POST pause|resume|test
// (internal/api/builders.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollMs } from '../lib/pollTier'
import { buildRunnerKeys } from './buildRunners'

export type BuilderStatus =
  'ready' | 'busy' | 'paused' | 'offline' | 'pressure' | 'draining'
export type BuilderPressure = 'ok' | 'warn' | 'critical' | 'unknown'

export interface Builder {
  id: string
  name: string
  status: BuilderStatus
  paused: boolean
  tier: string
  os: string
  arch: string
  agent_version: string
  cpu_cores: number
  pressure: BuilderPressure
  slots_used: number
  slots_total: number
  active_builds: number
  waiting: number
  accepts_app_workloads: boolean
  labels: string[] | null
  last_seen_at?: string
  last_build_at?: string
  recent_builds: number
  recent_failed: number
  avg_build_ms: number
  disk_free_bytes?: number
}

export interface BuilderBuild {
  app: string
  tag: string
  started_at: string
  duration_ms: number
  result: 'success' | 'failed'
}

export interface BuilderDetail extends Builder {
  builds: BuilderBuild[] | null
}

export interface BuilderTestResult {
  id: string
  name: string
  ok: boolean
  latency_ms: number
  arch: string
  os: string
  agent_version: string
  buildkit_version?: string
  disk_free_bytes?: number
  error?: string
}

export const builderKeys = {
  all: ['builders'] as const,
  one: (id: string) => ['builders', id] as const,
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

function url(id: string, action = ''): string {
  return `/api/v1/builders/${encodeURIComponent(id)}${action}`
}

export function buildersOptions(fastPoll = false, enabled = true) {
  return queryOptions({
    queryKey: builderKeys.all,
    queryFn: async ({ signal }) =>
      json<{ builders: Builder[] | null }>(
        await fetch('/api/v1/builders', { signal }),
        'fetch builders',
      ),
    retry: false,
    enabled,
    refetchInterval: pollMs(fastPoll ? 'live' : 'active'),
  })
}

// fastPoll is for the add dialog, which waits for a new builder to appear.
export function useBuilders(fastPoll = false, enabled = true) {
  return useQuery(buildersOptions(fastPoll, enabled))
}

export function useBuilder(id: string) {
  return useQuery({
    queryKey: builderKeys.one(id),
    queryFn: async ({ signal }) =>
      json<BuilderDetail>(await fetch(url(id), { signal }), 'fetch builder'),
    retry: false,
    refetchInterval: pollMs('active'),
  })
}

export function useSetBuilderPaused(id: string) {
  const queryClient = useQueryClient()
  return useMutation<Builder, ApiError, boolean>({
    meta: { errorToast: false },
    mutationFn: async (paused) =>
      json<Builder>(
        await fetch(url(id, paused ? '/pause' : '/resume'), { method: 'POST' }),
        'set builder paused',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: builderKeys.all })
      void queryClient.invalidateQueries({ queryKey: buildRunnerKeys.all })
    },
  })
}

// Never blocks the page: the caller shows the pending state inline.
export function useTestBuilder(id: string) {
  return useMutation<BuilderTestResult, ApiError, void>({
    meta: { errorToast: false },
    mutationFn: async () =>
      json<BuilderTestResult>(
        await fetch(url(id, '/test'), { method: 'POST' }),
        'test builder',
      ),
  })
}
