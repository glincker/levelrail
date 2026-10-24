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
  | 'rss'
  | 'threads'
  | 'telemetry_size'
  | 'reconcile_breakers'
  | 'reconcile_storm'
  | 'db_integrity'

export type CPHealthFix =
  | 'wal_checkpoint'
  | 'db_optimize'
  | 'reset_slow_log'
  | 'telemetry_prune'
  | 'reset_reconcile_guard'
  | 'integrity_check'
  | 'integrity_restore'

export interface CPHealthBreaker {
  name: string
  state: 'closed' | 'open' | 'half_open'
  failures: number
  open_until: string
  last_error?: string
  trips: number
  panics: number
  throttled: number
  skipped: number
}

export interface CPHealthGuard {
  passes_deferred: number
  recent_limited: number
  open_breakers: number
  breakers: CPHealthBreaker[]
}

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

export interface CPHealthLoop {
  name: string
  interval_seconds: number
  runs: number
  errors: number
  last_run_at: string
  last_duration_ms: number
  cost_percent: number
  running: boolean
  last_error?: string
}

export type IdleFootprintStatus =
  'within_tolerance' | 'over_tolerance' | 'tolerance_unset' | 'no_baseline'

export interface IdleFootprintCheck {
  name: 'rss' | 'cpu' | 'goroutines' | 'open_fds'
  unit: string
  current: number
  baseline: number
  limit: number
  ok: boolean
}

// Matches internal/footprint.Assessment.
export interface IdleFootprint {
  status: IdleFootprintStatus
  detail?: string
  platform: string
  current: {
    rss_bytes: number
    cpu_percent: number
    cpu_window_seconds: number
    goroutines: number
    open_fds: number
    threads: number
    apps: number
  }
  baseline?: { apps: number; peak_rss_mb: number }
  checks: IdleFootprintCheck[]
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
    read_pool_enabled: boolean
    read_pool_size: number
    read_in_use: number
    read_wait_count: number
    read_wait_ms: number
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
    guard: CPHealthGuard
  }
  streams: { active: number; cap: number }
  footprint: {
    rss_bytes: number
    threads: number
    cpu_percent: number
    telemetry_bytes: number
    memory_limit: { limit_bytes: number; source: string; percent: number }
    loops: CPHealthLoop[]
  }
  idle_footprint?: IdleFootprint
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
    mutationFn: async (req: {
      fix: CPHealthFix
      name?: string
    }): Promise<CPHealth> => {
      const res = await fetch(`${URL}/fix`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
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
