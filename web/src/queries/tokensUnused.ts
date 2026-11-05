import { queryOptions, useQuery } from '@tanstack/react-query'

// GET /api/v1/tokens/unused (internal/api/tokens_unused.go): the caller's own
// idle tokens. Items stay empty while tokens.unused_alert_days is 0 (off).
export interface UnusedToken {
  id: string
  name: string
  abilities: string[]
  created_at: string
  last_used_at?: string
}

export interface UnusedTokensResponse {
  alert_days: number
  items: UnusedToken[]
}

export const tokensUnusedKey = ['tokens', 'unused'] as const

async function fetchUnusedTokens(): Promise<UnusedTokensResponse> {
  const res = await fetch('/api/v1/tokens/unused')
  if (!res.ok) throw new Error(`unused tokens: ${String(res.status)}`)
  const body = (await res.json()) as Partial<UnusedTokensResponse>
  return { alert_days: body.alert_days ?? 0, items: body.items ?? [] }
}

export function tokensUnusedQueryOptions() {
  return queryOptions({
    queryKey: tokensUnusedKey,
    queryFn: fetchUnusedTokens,
  })
}

export function useUnusedTokens() {
  return useQuery(tokensUnusedQueryOptions())
}
