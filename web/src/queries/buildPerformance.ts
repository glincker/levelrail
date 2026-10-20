// Fetchers for /api/v1/apps/{name}/build-stats and /api/v1/build-limits
// (internal/api/build_performance.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type BuildRunStatus =
  'success' | 'failed' | 'timeout' | 'canceled' | 'memory_limit'

export interface BuildTrend {
  builds: number
  succeeded: number
  failed: number
  timed_out: number
  canceled: number
  skewed: number
  p50_seconds: number
  p95_seconds: number
  min_seconds: number
  max_seconds: number
  cache_hit_ratio: number
  cache_known: boolean
}

export interface BuildRun {
  started_at: string
  tag: string
  status: BuildRunStatus
  duration_seconds: number
  queue_wait_seconds: number
  cached_steps: number
  total_steps: number
  error?: string
}

export interface BuildStats {
  app: string
  limit: number
  trend: BuildTrend
  runs: BuildRun[]
}

export type BuildSpeedFixKind = 'auto' | 'guided' | 'manual'

export type BuildSpeedCode =
  'cache_disabled' | 'cache_unconfigured' | 'low_cache_hit' | 'slower_recently'

export interface BuildSpeedFinding {
  code: BuildSpeedCode
  severity: 'warning' | 'info'
  fix_kind: BuildSpeedFixKind
  target_id?: string
}

export interface BuildSpeed {
  builds: number
  warm_p50_seconds: number
  cold_p50_seconds: number
  warm_known: boolean
  time_saved_seconds: number
  findings: BuildSpeedFinding[]
}

export interface BuildLimitValues {
  max_concurrent: number
  timeout_seconds: number
  memory_mb: number
  cpu_millis: number
}

export interface BuildLimitsRequest {
  max_concurrent?: number
  timeout_seconds?: number
  memory_mb?: number
  cpu_millis?: number
}

export interface BuildLimits {
  effective: BuildLimitValues
  default: BuildLimitValues
  overrides: BuildLimitsRequest
}

export interface AppBuildLimits {
  effective: BuildLimitValues
  global: BuildLimitValues
  overrides: BuildLimitsRequest
}

export const buildPerformanceKeys = {
  stats: (app: string, limit: number) =>
    ['apps', app, 'build-stats', limit] as const,
  limits: () => ['build-limits'] as const,
  appLimits: (app: string) => ['apps', app, 'build-limits'] as const,
  speed: (app: string) => ['apps', app, 'build-speed'] as const,
}

async function requestJson<T>(
  input: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(input, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function buildStatsQueryOptions(app: string, limit = 20) {
  return queryOptions({
    queryKey: buildPerformanceKeys.stats(app, limit),
    queryFn: () =>
      requestJson<BuildStats>(
        `/api/v1/apps/${encodeURIComponent(app)}/build-stats?limit=${limit}`,
        undefined,
        'fetch build stats',
      ),
  })
}

export function useBuildStats(app: string, limit = 20) {
  return useQuery(buildStatsQueryOptions(app, limit))
}

export function useBuildSpeed(app: string) {
  return useQuery(
    queryOptions({
      queryKey: buildPerformanceKeys.speed(app),
      queryFn: () =>
        requestJson<BuildSpeed>(
          `/api/v1/apps/${encodeURIComponent(app)}/build-speed`,
          undefined,
          'fetch build speed',
        ),
    }),
  )
}

export function buildLimitsQueryOptions() {
  return queryOptions({
    queryKey: buildPerformanceKeys.limits(),
    queryFn: () =>
      requestJson<BuildLimits>(
        '/api/v1/build-limits',
        undefined,
        'fetch build limits',
      ),
  })
}

export function useBuildLimits() {
  return useQuery(buildLimitsQueryOptions())
}

export function useSetBuildLimits() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: BuildLimitsRequest) =>
      requestJson<BuildLimits>(
        '/api/v1/build-limits',
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
        'save build limits',
      ),
    onSuccess: (data) => {
      qc.setQueryData(buildPerformanceKeys.limits(), data)
    },
  })
}

export function useResetBuildLimits() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      requestJson<BuildLimits>(
        '/api/v1/build-limits',
        { method: 'DELETE' },
        'reset build limits',
      ),
    onSuccess: (data) => {
      qc.setQueryData(buildPerformanceKeys.limits(), data)
    },
  })
}

const appLimitsUrl = (app: string) =>
  `/api/v1/apps/${encodeURIComponent(app)}/build-limits`

export function useAppBuildLimits(app: string) {
  return useQuery(
    queryOptions({
      queryKey: buildPerformanceKeys.appLimits(app),
      queryFn: () =>
        requestJson<AppBuildLimits>(
          appLimitsUrl(app),
          undefined,
          'fetch app build limits',
        ),
    }),
  )
}

export function useSetAppBuildLimits(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: BuildLimitsRequest) =>
      requestJson<AppBuildLimits>(
        appLimitsUrl(app),
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
        'save app build limits',
      ),
    onSuccess: (data) => {
      qc.setQueryData(buildPerformanceKeys.appLimits(app), data)
    },
  })
}

export function useResetAppBuildLimits(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      requestJson<AppBuildLimits>(
        appLimitsUrl(app),
        { method: 'DELETE' },
        'reset app build limits',
      ),
    onSuccess: (data) => {
      qc.setQueryData(buildPerformanceKeys.appLimits(app), data)
    },
  })
}
