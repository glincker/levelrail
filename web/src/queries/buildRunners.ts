// Build runners: GET /api/v1/builds/runners, the per-app preference routes
// /api/v1/apps/{name}/build-runner and the build row (internal/api/build_runners.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollMs } from '../lib/pollTier'

export type RunnerKind = 'control_plane' | 'node'
export type RunnerStatus =
  'ready' | 'busy' | 'pressure' | 'draining' | 'offline'

export type RunnerMode = 'auto' | 'control_plane' | 'node'

export interface BuildRunner {
  id: string
  name: string
  kind: RunnerKind
  status: RunnerStatus
  health: string
  slots_used: number
  slots_total: number
  active_builds?: number
  waiting: number
  load: number
  pressure: string
  architecture?: string
  cpu_cores?: number
  labels: string[] | null
  last_seen_at?: string
  last_build_at?: string
  last_build_app?: string
  last_build_status?: string
  cache_bytes?: number
  trust?: string
  skip_reason?: string
}

export interface BuildRunnersView {
  strategy: string
  runners: BuildRunner[] | null
}

export interface AppBuildRunner {
  app?: string
  mode?: RunnerMode
  node_id: string
  pinned: boolean
  labels: string[] | null
}

export interface AppBuildRunnerInput {
  mode: RunnerMode
  node_id: string
  pinned: boolean
  labels: string[]
}

// The build row fields the run view header reads.
export interface BuildRunnerInfo {
  runner_name?: string
  runner_reason?: string
  queue_wait_ms?: number
}

export const buildRunnerKeys = {
  all: ['build-runners'] as const,
  app: (name: string) => ['build-runners', 'app', name] as const,
  row: (id: string) => ['build-runners', 'row', id] as const,
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

export function buildRunnersOptions() {
  return queryOptions({
    queryKey: buildRunnerKeys.all,
    queryFn: async ({ signal }) =>
      json<BuildRunnersView>(
        await fetch('/api/v1/builds/runners', { signal }),
        'fetch build runners',
      ),
    retry: false,
    refetchInterval: pollMs('active'),
  })
}

export function useBuildRunners() {
  return useQuery(buildRunnersOptions())
}

function prefUrl(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/build-runner`
}

export function useAppBuildRunner(app: string) {
  return useQuery({
    queryKey: buildRunnerKeys.app(app),
    queryFn: async ({ signal }) =>
      json<AppBuildRunner>(
        await fetch(prefUrl(app), { signal }),
        'fetch build runner preference',
      ),
    retry: false,
  })
}

export function useSaveAppBuildRunner(app: string) {
  const queryClient = useQueryClient()
  return useMutation<AppBuildRunner, ApiError, AppBuildRunnerInput>({
    mutationFn: async (input) =>
      json<AppBuildRunner>(
        await fetch(prefUrl(app), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        }),
        'save build runner preference',
      ),
    onSuccess: (view) => {
      queryClient.setQueryData(buildRunnerKeys.app(app), view)
    },
  })
}

export function useClearAppBuildRunner(app: string) {
  const queryClient = useQueryClient()
  return useMutation<AppBuildRunner, ApiError, void>({
    mutationFn: async () =>
      json<AppBuildRunner>(
        await fetch(prefUrl(app), { method: 'DELETE' }),
        'clear build runner preference',
      ),
    onSuccess: (view) => {
      queryClient.setQueryData(buildRunnerKeys.app(app), view)
    },
  })
}

// Image-only deploys have no build row; a 404 is "nothing to show", not an error.
export function useBuildRunnerInfo(deployId: string, polling: boolean) {
  return useQuery({
    queryKey: buildRunnerKeys.row(deployId),
    queryFn: async ({ signal }): Promise<BuildRunnerInfo | null> => {
      const res = await fetch(
        `/api/v1/builds/${encodeURIComponent(deployId)}`,
        {
          signal,
        },
      )
      if (res.status === 404) return null
      return json<BuildRunnerInfo>(res, 'fetch build')
    },
    retry: false,
    refetchInterval: polling ? pollMs('active') : false,
  })
}
