import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface TrafficAreaSummary {
  attention?: number
}

// Assumed wire shape of GET /api/v1/traffic/summary; unknown fields are ignored.
export interface TrafficSummary {
  domains?: TrafficAreaSummary
  dns?: TrafficAreaSummary
  proxy?: TrafficAreaSummary
}

export const trafficSummaryKeys = {
  all: ['traffic', 'summary'] as const,
}

const MISSING_ENDPOINT_STATUSES = new Set([404, 405, 501])

// A missing endpoint resolves to null so the sidebar just shows no badge.
export async function fetchTrafficSummary(): Promise<TrafficSummary | null> {
  const res = await fetch('/api/v1/traffic/summary')
  if (MISSING_ENDPOINT_STATUSES.has(res.status)) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch traffic summary failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as TrafficSummary
}

export function trafficSummaryQueryOptions() {
  return queryOptions({
    queryKey: trafficSummaryKeys.all,
    queryFn: fetchTrafficSummary,
    staleTime: 30_000,
    retry: false,
  })
}

export function useTrafficSummary() {
  return useQuery(trafficSummaryQueryOptions())
}

export interface TrafficAttentionCounts {
  domains: number
  dns: number
  proxy: number
}

function count(area: TrafficAreaSummary | undefined): number {
  const n = area?.attention
  return typeof n === 'number' && Number.isFinite(n) && n > 0
    ? Math.floor(n)
    : 0
}

/** Per-area counts of items that need a person, zero when unknown. */
export function attentionCounts(
  summary: TrafficSummary | null | undefined,
): TrafficAttentionCounts {
  return {
    domains: count(summary?.domains),
    dns: count(summary?.dns),
    proxy: count(summary?.proxy),
  }
}
