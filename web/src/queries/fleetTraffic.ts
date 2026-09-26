import { queryOptions, useQueries, useQuery } from '@tanstack/react-query'
import type { RequestSeries } from '../types/requests'
import type { DeployAttempt } from '../types/deployAttempt'
import { fetchRequestSeries } from './requests'
import {
  deployAttemptsQueryOptions,
  fetchDeployAttempts,
} from './deployAttempts'

const WINDOW_MS = 60 * 60 * 1000
const STEP = '5m'
export const TRAFFIC_STALE_MS = 30_000
const MAX_INFLIGHT = 4

// The API has no batched per-app traffic endpoint, so row and fleet reads
// share this queue to keep the browser at a few requests in flight.
let inflight = 0
const waiting: (() => void)[] = []

export async function withLimit<T>(task: () => Promise<T>): Promise<T> {
  if (inflight >= MAX_INFLIGHT) {
    await new Promise<void>((resolve) => waiting.push(resolve))
  }
  inflight += 1
  try {
    return await task()
  } finally {
    inflight -= 1
    waiting.shift()?.()
  }
}

export const trafficKeys = {
  app: (name: string) => ['fleet', 'traffic', name] as const,
}

export function appTrafficQueryOptions(name: string) {
  return queryOptions({
    queryKey: trafficKeys.app(name),
    queryFn: (): Promise<RequestSeries> => {
      const to = new Date()
      return withLimit(() =>
        fetchRequestSeries(name, {
          from: new Date(to.getTime() - WINDOW_MS),
          to,
          step: STEP,
        }),
      )
    },
    staleTime: TRAFFIC_STALE_MS,
    refetchInterval: 60_000,
    retry: false,
  })
}

export function useAppTraffic(name: string, enabled = true) {
  return useQuery({ ...appTrafficQueryOptions(name), enabled })
}

export function useFleetTraffic(names: string[]) {
  const results = useQueries({
    queries: names.map((n) => appTrafficQueryOptions(n)),
  })
  return {
    series: results.map((r) => r.data),
    isPending: names.length > 0 && results.every((r) => r.isPending),
  }
}

export function useAppLastDeploy(name: string, enabled = true) {
  return useQuery({
    ...deployAttemptsQueryOptions(name),
    queryFn: () =>
      withLimit((): Promise<DeployAttempt[]> => fetchDeployAttempts(name)),
    staleTime: 60_000,
    enabled,
    retry: false,
  })
}

export function useFleetDeploys(names: string[]) {
  const results = useQueries({
    queries: names.map((n) => ({
      ...deployAttemptsQueryOptions(n),
      queryFn: () =>
        withLimit((): Promise<DeployAttempt[]> => fetchDeployAttempts(n)),
      staleTime: 30_000,
      retry: false,
    })),
  })
  const byApp: Record<string, DeployAttempt[] | undefined> = {}
  names.forEach((n, i) => {
    byApp[n] = results[i]?.data
  })
  return {
    byApp,
    isPending: names.length > 0 && results.every((r) => r.isPending),
  }
}
