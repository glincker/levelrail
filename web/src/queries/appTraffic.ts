// Overview traffic tiles: one GET /apps/{name}/requests over two windows,
// the server returns the window summary and the previous equal window for deltas.

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { appKeys } from './apps'
import { fetchRequestSeries } from './requests'
import type { RequestSeries, RequestSummary } from '../types/requests'
import {
  TRAFFIC_REFETCH_MS,
  TRAFFIC_WINDOW_MINUTES,
} from '../components/overview/config'

const MS_PER_MINUTE = 60_000
const STEP = '60s'

export interface WindowStats {
  requests: number
  ratePerSec: number
  /** 4xx plus 5xx as a 0 to 1 share of requests (the Apps list counts 5xx only). */
  errorRate: number
  p95Ms: number
}

export interface TrafficStats {
  hasTraffic: boolean
  current: WindowStats
  previous: WindowStats
  rateSeries: number[]
  errorSeries: number[]
  p95Series: number[]
}

const EMPTY_WINDOW: WindowStats = {
  requests: 0,
  ratePerSec: 0,
  errorRate: 0,
  p95Ms: 0,
}

// Server summaries come from merged histograms, so p95 is a real percentile
// of the window rather than an average of per-minute values.
function summaryStats(s: RequestSummary | undefined): WindowStats {
  if (!s) return EMPTY_WINDOW
  return {
    requests: s.requests,
    ratePerSec: s.rate_per_sec,
    errorRate: s.error_rate_4xx + s.error_rate_5xx,
    p95Ms: s.p95_ms,
  }
}

export function computeTraffic(series: RequestSeries): TrafficStats {
  const points = series.points ?? []
  const cur = summaryStats(series.summary)
  const prev = summaryStats(series.previous_summary)
  return {
    hasTraffic: cur.requests + prev.requests > 0,
    current: cur,
    previous: prev,
    rateSeries: points.map((p) => p.rate_per_sec),
    errorSeries: points.map((p) => (p.error_rate_4xx + p.error_rate_5xx) * 100),
    p95Series: points.map((p) => p.p95_ms),
  }
}

/**
 * Percent change versus the previous window. Undefined without a baseline or
 * when either window has fewer requests than minSample, so tiny samples never
 * produce a huge badge.
 */
export function percentChange(
  current: number,
  previous: number,
  samples?: { current: number; previous: number; minSample: number },
): number | undefined {
  if (previous <= 0) return undefined
  if (
    samples &&
    (samples.current < samples.minSample ||
      samples.previous < samples.minSample)
  ) {
    return undefined
  }
  return ((current - previous) / previous) * 100
}

export const trafficKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'overview-traffic'] as const,
}

export function useAppTraffic(appName: string) {
  return useQuery({
    queryKey: trafficKeys.detail(appName),
    queryFn: async (): Promise<TrafficStats> => {
      const now = Date.now()
      const series = await fetchRequestSeries(appName, {
        from: new Date(now - TRAFFIC_WINDOW_MINUTES * MS_PER_MINUTE),
        to: new Date(now),
        step: STEP,
        compare: true,
      })
      return computeTraffic(series)
    },
    refetchInterval: TRAFFIC_REFETCH_MS,
    retry: false,
    placeholderData: keepPreviousData,
  })
}
