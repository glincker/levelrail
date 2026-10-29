import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type RehydrateItemStatus =
  'planned' | 'running' | 'succeeded' | 'failed' | 'skipped' | 'blocked'

export type RehydrateState =
  'planned' | 'running' | 'succeeded' | 'partial' | 'failed'

export interface RehydrateBackup {
  id: string
  finished_at: string
  size_bytes: number
  age_seconds: number
}

export interface RehydrateItem {
  id: string
  kind: 'database' | 'volume'
  app?: string
  volume?: string
  database?: string
  engine?: string
  status: RehydrateItemStatus
  reason?: string
  backup?: RehydrateBackup
  restore_id?: string
  error?: string
}

export interface RehydrateSummary {
  total: number
  restorable: number
  succeeded: number
  failed: number
  skipped: number
  blocked: number
  worst_backup_age_seconds: number
}

export interface RehydrateReport {
  id: string
  mode: 'plan' | 'run'
  state: RehydrateState
  generated_at: string
  finished_at?: string
  max_backup_age_seconds: number
  concurrency: number
  allow_stale: boolean
  items: RehydrateItem[]
  summary: RehydrateSummary
}

const BASE = '/api/v1/control-plane/rehydrate'

export const rehydrateKeys = {
  report: () => ['control-plane-rehydrate', 'report'] as const,
}

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(BASE + path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function post(allowStale: boolean): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ allow_stale: allowStale }),
  }
}

// A 404 means no run has happened yet, which is a normal empty state.
export function useRehydrateReport() {
  return useQuery({
    queryKey: rehydrateKeys.report(),
    queryFn: async (): Promise<RehydrateReport | null> => {
      try {
        return await request<RehydrateReport>(
          '/report',
          undefined,
          'load report',
        )
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) return null
        throw err
      }
    },
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.state === 'running' ? 2000 : false,
  })
}

export function useRehydratePlan() {
  return useMutation<RehydrateReport, Error, boolean>({
    mutationFn: (allowStale) =>
      request<RehydrateReport>('/plan', post(allowStale), 'plan'),
  })
}

export function useRehydrateRun() {
  const qc = useQueryClient()
  return useMutation<RehydrateReport, Error, boolean>({
    mutationFn: (allowStale) =>
      request<RehydrateReport>('/run', post(allowStale), 'start recovery'),
    onSuccess: () => qc.invalidateQueries({ queryKey: rehydrateKeys.report() }),
  })
}
