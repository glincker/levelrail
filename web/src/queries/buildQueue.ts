// Build queue controls: GET /api/v1/builds/queue, POST .../pause|drain|resume
// and POST /api/v1/builds/kill-all (internal/api/build_queue.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollMs } from '../lib/pollTier'
import { deploymentKeys } from './deployments'
import type { StepUpScope } from './envProtection'

export type BuildQueueState = 'running' | 'paused' | 'draining'
export type BuildPhase = 'preparing' | 'held' | 'waiting' | 'building'

export interface BuildQueueRunning {
  deploy_id?: string
  app: string
  node?: string
  phase: BuildPhase
  started_at?: string
  elapsed_seconds: number
}

export interface BuildQueueQueued {
  deploy_id: string
  app: string
  position: number
  queued_at?: string
  wait_seconds: number
  wait_reason?: string
}

export interface BuildQueueChange {
  state: string
  reason?: string
  actor?: string
  at: string
}

export interface BuildQueue {
  state: BuildQueueState
  reason?: string
  actor?: string
  since?: string
  auto_resume_at?: string
  drained: boolean
  slots: { used: number; total: number }
  running: BuildQueueRunning[]
  queued: BuildQueueQueued[]
  recent: BuildQueueChange[]
}

export interface KillAllItem {
  deploy_id: string
  app: string
  was: string
  error?: string
}

export interface KillAllResult {
  canceled: KillAllItem[]
  failed: KillAllItem[]
  paused: boolean
}

export interface KillAllInput {
  reason?: string
  pause?: boolean
}

export const BUILD_QUEUE_REASON_MAX = 500

// Scope of the password step-up the kill-all endpoint asks for.
export const buildQueueKillAllStepUp: StepUpScope = {
  action: 'manage',
  app: 'build-queue-kill-all',
}

export const buildQueueKeys = {
  all: ['build-queue'] as const,
}

async function send<T>(url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method: body === undefined ? 'GET' : 'POST',
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `build queue request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function buildQueueOptions() {
  return queryOptions({
    queryKey: buildQueueKeys.all,
    queryFn: async () => {
      const view = await send<BuildQueue>('/api/v1/builds/queue')
      // A proxy page or older server must not render as a broken queue.
      if (!view || !view.slots || !Array.isArray(view.running)) {
        throw new ApiError(502, 'unexpected build queue response')
      }
      return view
    },
    retry: false,
    // react-query skips interval refetches while the tab is hidden.
    refetchInterval: pollMs('active'),
  })
}

export function useBuildQueue() {
  return useQuery(buildQueueOptions())
}

export type QueueAction = 'pause' | 'drain' | 'resume'

export function useQueueAction() {
  const queryClient = useQueryClient()
  return useMutation<
    BuildQueue,
    ApiError,
    { action: QueueAction; reason?: string }
  >({
    mutationFn: ({ action, reason }) =>
      send<BuildQueue>(`/api/v1/builds/queue/${action}`, {
        reason: reason?.trim() || undefined,
      }),
    onSuccess: (view) => {
      queryClient.setQueryData(buildQueueKeys.all, view)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: buildQueueKeys.all })
    },
  })
}

export function useKillAllBuilds() {
  const queryClient = useQueryClient()
  return useMutation<KillAllResult, ApiError, KillAllInput>({
    mutationFn: (input) =>
      send<KillAllResult>('/api/v1/builds/kill-all', {
        reason: input.reason?.trim() || undefined,
        pause: input.pause || undefined,
      }),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: buildQueueKeys.all })
      void queryClient.invalidateQueries({ queryKey: deploymentKeys.all })
      void queryClient.invalidateQueries({ queryKey: ['apps'] })
    },
  })
}
