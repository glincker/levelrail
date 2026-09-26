// Overview traffic tiles: one GET /apps/{name}/requests over two windows,
// split client-side into "now" and "previous" so each tile can show a delta.

import { useQuery } from '@tanstack/react-query'
import { appKeys } from './apps'
import { fetchRequestSeries } from './requests'
import type { RequestPoint } from '../types/requests'
import {
  TRAFFIC_REFETCH_MS,
  TRAFFIC_WINDOW_MINUTES,
} from '../components/overview/config'

const MS_PER_MINUTE = 60_000
const STEP = '60s'

export interface WindowStats {
  requests: number
  ratePerSec: number
  /** 4xx plus 5xx as a 0 to 1 share of requests. */
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

function windowStats(points: RequestPoint[]): WindowStats {
  const requests = points.reduce((s, p) => s + p.requests, 0)
  const ratePerSec = points.length
    ? points.reduce((s, p) => s + p.rate_per_sec, 0) / points.length
    : 0
  const errors = points.reduce(
    (s, p) => s + p.requests * (p.error_rate_4xx + p.error_rate_5xx),
    0,
  )
  const p95 = points.reduce((s, p) => s + p.requests * p.p95_ms, 0)
  return {
    requests,
    ratePerSec,
    errorRate: requests > 0 ? errors / requests : 0,
    p95Ms: requests > 0 ? p95 / requests : 0,
  }
}

export function computeTraffic(
  points: RequestPoint[],
  windowMinutes: number,
  nowMs: number,
): TrafficStats {
  const cutoff = nowMs - windowMinutes * MS_PER_MINUTE
  const current = points.filter((p) => Date.parse(p.timestamp) > cutoff)
  const previous = points.filter((p) => Date.parse(p.timestamp) <= cutoff)
  const cur = windowStats(current)
  const prev = windowStats(previous)
  return {
    hasTraffic: cur.requests + prev.requests > 0,
    current: cur,
    previous: prev,
    rateSeries: current.map((p) => p.rate_per_sec),
    errorSeries: current.map(
      (p) => (p.error_rate_4xx + p.error_rate_5xx) * 100,
    ),
    p95Series: current.map((p) => p.p95_ms),
  }
}

/** Percent change versus the previous window, undefined without a baseline. */
export function percentChange(
  current: number,
  previous: number,
): number | undefined {
  if (previous <= 0) return undefined
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
        from: new Date(now - 2 * TRAFFIC_WINDOW_MINUTES * MS_PER_MINUTE),
        to: new Date(now),
        step: STEP,
      })
      return computeTraffic(series.points ?? [], TRAFFIC_WINDOW_MINUTES, now)
    },
    refetchInterval: TRAFFIC_REFETCH_MS,
    retry: false,
  })
}
