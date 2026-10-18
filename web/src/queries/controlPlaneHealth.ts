import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type CPHealthSeverity = 'warn' | 'crit'

export type CPHealthFindingId =
  | 'goroutines'
  | 'heap'
  | 'db_latency'
  | 'db_queue'
  | 'wal_size'
  | 'slow_queries'
  | 'reconcile_pass'
  | 'event_lag'
  | 'streams'

export type CPHealthFix = 'wal_checkpoint' | 'db_optimize' | 'reset_slow_log'

export interface CPHealthFinding {
  id: CPHealthFindingId
  severity: CPHealthSeverity
  value: number
  threshold: number
  fix?: CPHealthFix
}

export interface CPHealthSlowQuery {
  sql: string
  duration_ms: number
  at: string
}

// Matches internal/api's cpHealthResource.
export interface CPHealth {
  checked_at: string
  status: 'ok' | CPHealthSeverity
  runtime: {
    goroutines: number
    heap_bytes: number
    gc_pause_ms: number
    cpus: number
  }
  db: {
    queries: number
    slow_queries: number
    slow_threshold_ms: number
    p50_ms: number
    p95_ms: number
    p99_ms: number
    in_use: number
    wait_count: number
    wait_ms: number
    wal_bytes: number
    db_bytes: number
    recent_slow: CPHealthSlowQuery[]
  }
  reconcile: {
    passes: number
    controllers: number
    last_pass_ms: number
    p50_ms: number
    p95_ms: number
    slowest_controller: string
    lag_seconds: number
    pending: boolean
  }
  streams: { active: number; cap: number }
  findings: CPHealthFinding[]
}

const URL = '/api/v1/system/control-plane-health'
const KEY = ['control-plane-health']

export function useControlPlaneHealth() {
  return useQuery({
    queryKey: KEY,
    queryFn: async ({ signal }): Promise<CPHealth> => {
      const res = await fetch(URL, { signal, cache: 'no-store' })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch control plane health failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as CPHealth
    },
    refetchInterval: 10_000,
  })
}

export function useApplyControlPlaneFix() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (fix: CPHealthFix): Promise<CPHealth> => {
      const res = await fetch(`${URL}/fix`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ fix }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `control plane fix failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as CPHealth
    },
    onSuccess: (data) => {
      qc.setQueryData(KEY, data)
    },
  })
}
