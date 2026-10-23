import { queryOptions, useQuery } from '@tanstack/react-query'
import {
  activeCliTokens,
  cliTokenStatus,
  listCliTokens,
} from '../lib/cliConnection'
import { ApiError } from '../lib/apiError'
import type { TokenResource } from '../types/token'

// GET /api/v1/auth/my-credentials (internal/api/account_credentials.go):
// the caller's own live sessions and active tokens, never a secret.
export interface MyCredentials {
  sessions: unknown[]
  tokens: TokenResource[]
}

export const myCredentialsKey = ['account', 'my-credentials'] as const

const REFETCH_MS = Number(import.meta.env.VITE_CLI_STATUS_REFETCH_MS ?? 30000)

export async function fetchMyCredentials(): Promise<MyCredentials> {
  const res = await fetch('/api/v1/auth/my-credentials', { cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(res.status, `my credentials failed: ${res.status}`)
  }
  const body = (await res.json()) as Partial<MyCredentials>
  return { sessions: body.sessions ?? [], tokens: body.tokens ?? [] }
}

export function myCredentialsQueryOptions() {
  return queryOptions({
    queryKey: myCredentialsKey,
    queryFn: fetchMyCredentials,
    retry: false,
    staleTime: REFETCH_MS,
    refetchInterval: REFETCH_MS,
  })
}

// Select runs outside render, so the expiry check can read the clock; the
// periodic refetch keeps the answer fresh.
export function useCliTokens() {
  return useQuery({
    ...myCredentialsQueryOptions(),
    select: (data) => activeCliTokens(data.tokens, Date.now()),
  })
}

export function useCliTokenRows(enabled: boolean) {
  return useQuery({
    ...myCredentialsQueryOptions(),
    enabled,
    select: (data) => {
      const now = Date.now()
      return listCliTokens(data.tokens).map((token) => ({
        token,
        status: cliTokenStatus(token, now),
      }))
    },
  })
}
