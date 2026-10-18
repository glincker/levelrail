import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/chaos.Result.
export interface ChaosResult {
  id?: string
  scenario: string
  status: 'passed' | 'failed' | 'skipped'
  summary: string
  recovery_ms: number
  error_window_ms: number
  requests: number
  request_errors: number
  hardware?: string
  ran_at: string
}

// Matches internal/api's chaosReport.
export interface ResilienceReport {
  scenarios: { name: string; title: string; expectation: string }[]
  latest: ChaosResult[]
  not_yet_measured: string[]
}

export async function fetchResilienceReport(
  signal?: AbortSignal,
): Promise<ResilienceReport> {
  const res = await fetch('/api/v1/resilience', { signal, cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch resilience report failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ResilienceReport
}

export function useResilienceReport() {
  return useQuery({
    queryKey: ['resilience-report'],
    queryFn: ({ signal }) => fetchResilienceReport(signal),
    staleTime: 60_000,
  })
}
