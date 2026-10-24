// Fetchers for /api/v1/system/janitor (internal/api/janitor.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type JanitorStageName =
  | 'containers'
  | 'images'
  | 'build_cache'
  | 'volumes'
  | 'deploy_attempts'
  | 'api_tokens'

export const JANITOR_STAGES: JanitorStageName[] = [
  'containers',
  'images',
  'build_cache',
  'volumes',
  'deploy_attempts',
  'api_tokens',
]

export interface JanitorPolicy {
  enabled: boolean
  dry_run: boolean
  interval_minutes: number
  disk_free_budget_percent: number
  build_cache_budget_mb: number
  build_cache_max_age_hours: number
  volume_quarantine_days: number
  delete_orphan_volumes: boolean
  attempt_retention_days: number
  attempt_keep_per_app: number
  disabled_stages: JanitorStageName[]
}

export interface JanitorItem {
  name: string
  detail?: string
  size_bytes: number
  /** Rows covered by the item, for stages that remove rows rather than bytes. */
  count?: number
  action:
    'removed' | 'would_remove' | 'quarantined' | 'would_quarantine' | 'kept'
}

export interface JanitorStage {
  stage: JanitorStageName
  status: 'ran' | 'skipped' | 'failed'
  reason?: string
  items: JanitorItem[]
  reclaimed_bytes: number
  error?: string
}

export interface JanitorReport {
  id: string
  started_at: string
  trigger: 'scheduled' | 'manual' | 'disk_fix'
  dry_run: boolean
  free_percent: number
  pressure: boolean
  stages: JanitorStage[]
  reclaimed_bytes: number
  error?: string
}

export interface JanitorQuarantined {
  name: string
  size_bytes: number
  first_seen: string
  deletable_at: string
  delete_enabled: boolean
  ready: boolean
}

export interface JanitorStatus {
  policy: JanitorPolicy
  free_percent: number
  free_bytes: number
  total_bytes: number
  pressure: boolean
  last_run?: JanitorReport
  quarantine: JanitorQuarantined[]
  total_reclaimed_bytes: number
}

export const janitorKeys = {
  status: ['janitor', 'status'] as const,
  runs: ['janitor', 'runs'] as const,
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

async function fetchStatus(): Promise<JanitorStatus | null> {
  const res = await fetch('/api/v1/system/janitor')
  if (res.status === 501) return null
  if (!res.ok) throw await failure(res, 'load cleanup status')
  return (await res.json()) as JanitorStatus
}

export function useJanitorStatus() {
  return useQuery({ queryKey: janitorKeys.status, queryFn: fetchStatus })
}

async function fetchRuns(): Promise<JanitorReport[]> {
  const res = await fetch('/api/v1/system/janitor/runs?limit=20')
  if (!res.ok) throw await failure(res, 'load cleanup history')
  return (await res.json()) as JanitorReport[]
}

export function useJanitorRuns() {
  return useQuery({ queryKey: janitorKeys.runs, queryFn: fetchRuns })
}

export function useSaveJanitorPolicy() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (policy: JanitorPolicy): Promise<JanitorPolicy> => {
      const res = await fetch('/api/v1/system/janitor/policy', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(policy),
      })
      if (!res.ok) throw await failure(res, 'save cleanup policy')
      return (await res.json()) as JanitorPolicy
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['janitor'] })
    },
  })
}

export function useRunJanitor() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (dryRun: boolean): Promise<JanitorReport> => {
      const res = await fetch(
        `/api/v1/system/janitor/run?dry_run=${dryRun ? 'true' : 'false'}`,
        { method: 'POST' },
      )
      if (!res.ok) throw await failure(res, 'run cleanup')
      return (await res.json()) as JanitorReport
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['janitor'] })
    },
  })
}
