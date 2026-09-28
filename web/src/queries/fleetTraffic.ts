import { useQueries } from '@tanstack/react-query'
import type { DeployAttempt } from '../types/deployAttempt'
import {
  deployAttemptsQueryOptions,
  fetchDeployAttempts,
} from './deployAttempts'

const MAX_INFLIGHT = 4

// Per-app deploy reads share this queue to keep the browser at a few
// requests in flight.
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
