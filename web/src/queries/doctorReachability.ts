// GET /api/v1/doctor/reachability: per-port local listen state and the
// optional dial-back result. Fetched on demand, never on an interval.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { DEDUPE_STALE_MS } from '../lib/queryStale'

export type ReachabilityExternal =
  'not_checked' | 'reachable' | 'unreachable' | 'unknown'

export interface ReachabilityProbe {
  address: string
  listening: boolean
}

export interface ReachabilityPort {
  name: string
  port: number
  loopback_only?: boolean
  listening: boolean
  probes: ReachabilityProbe[] | null
  external: ReachabilityExternal
  /** Plain-words next step, set on a blocked port. */
  advice?: string
}

export interface ReachabilityReport {
  status: 'ok' | 'warn' | 'fail' | 'unknown'
  message: string
  ports: ReachabilityPort[] | null
  dialback_enabled: boolean
  dialback_error?: string
  firewall?: string
}

export const doctorReachabilityKeys = {
  all: ['doctor-reachability'] as const,
}

export async function fetchDoctorReachability(): Promise<ReachabilityReport> {
  const res = await fetch('/api/v1/doctor/reachability')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch reachability failed: ${res.status}`),
    )
  }
  return (await res.json()) as ReachabilityReport
}

export function doctorReachabilityQueryOptions() {
  return queryOptions({
    queryKey: doctorReachabilityKeys.all,
    queryFn: fetchDoctorReachability,
    staleTime: DEDUPE_STALE_MS,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  })
}

export function useDoctorReachability() {
  return useQuery(doctorReachabilityQueryOptions())
}
