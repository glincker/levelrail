import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { ChaosResult } from './resilience'

export type BenchmarkGroup = 'failover' | 'deploy' | 'rollout' | 'throughput'

// Matches internal/store's BenchmarkRunSummary.
export interface BenchmarkRunSummary {
  id: string
  generated_at: string
  version: string
  commit: string
  dirty: boolean
  host_summary: string
  host_os: string
  repeat: number
  passed: number
  failed: number
  skipped: number
  recorded_by?: string
  created_at: string
}

export interface BenchmarkList {
  runs: BenchmarkRunSummary[]
  keep: number
}

export interface BenchmarkResult extends ChaosResult {
  metrics?: Record<string, number>
}

export interface BenchmarkHost {
  os: string
  arch: string
  os_version?: string
  kernel?: string
  cpu_model?: string
  cpus: number
  mem_bytes?: number
  docker: { version?: string; os?: string; mem_bytes?: number }
  load1_start: number
  load1_end: number
  note?: string
}

// Matches internal/benchreport.Report.
export interface BenchmarkReport {
  schema_version: number
  generated_at: string
  build: { version: string; commit: string; dirty: boolean }
  host: BenchmarkHost
  repeat: number
  methodology: string[]
  caveats: string[]
  results: BenchmarkResult[]
  limits: { question: string; status: string }[]
}

export interface BenchmarkRun {
  run: BenchmarkRunSummary
  report: BenchmarkReport
  groups: Record<string, BenchmarkGroup>
}

export type DeltaDirection = 'better' | 'worse' | 'same' | ''

// Matches internal/benchreport.Delta.
export interface BenchmarkDelta {
  scenario: string
  group: BenchmarkGroup
  metric: string
  base: number | null
  head: number | null
  delta: number | null
  delta_pct: number | null
  direction?: DeltaDirection
}

interface CompareSide {
  id: string
  generated_at: string
  version: string
  commit: string
  dirty: boolean
  host: string
  runs: number
}

// Matches internal/benchreport.Comparison.
export interface BenchmarkComparison {
  base: CompareSide
  head: CompareSide
  deltas: BenchmarkDelta[]
  status_changes: { scenario: string; base: string; head: string }[]
  warnings: string[]
  noise_pct: number
  statistic: string
}

const URL = '/api/v1/benchmarks'
const KEY = ['benchmarks']

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, { signal, cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useBenchmarkRuns() {
  return useQuery({
    queryKey: KEY,
    queryFn: ({ signal }) => getJSON<BenchmarkList>(URL, signal),
    staleTime: 30_000,
  })
}

export function useBenchmarkRun(id: string | null) {
  return useQuery({
    queryKey: [...KEY, 'run', id],
    queryFn: ({ signal }) =>
      getJSON<BenchmarkRun>(`${URL}/${encodeURIComponent(id ?? '')}`, signal),
    enabled: id !== null,
    staleTime: Infinity,
  })
}

export function useBenchmarkComparison(
  base: string | null,
  head: string | null,
) {
  return useQuery({
    queryKey: [...KEY, 'compare', base, head],
    queryFn: ({ signal }) =>
      getJSON<BenchmarkComparison>(
        `${URL}/compare?base=${encodeURIComponent(base ?? '')}&head=${encodeURIComponent(head ?? '')}`,
        signal,
      ),
    enabled: base !== null && head !== null,
    staleTime: Infinity,
  })
}

export function useDeleteBenchmark() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string): Promise<void> => {
      const res = await fetch(`${URL}/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `delete benchmark failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}
