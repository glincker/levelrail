// Query-key factory and fetchers for the /auth/tokens resource
// (internal/api/tokens.go), following the same shared-queryOptions
// pattern queries/apps.ts and queries/deploys.ts already established:
// one options object per query, used by both a route loader's
// queryClient.ensureQueryData call and the component's
// useSuspenseQuery/hook call.
//
// All three endpoints here are session-only (tokens.go's own doc
// comments on handleCreateToken/handleListTokens/handleRevokeToken): a
// bearer token can never mint, list, or revoke another token. That's a
// server-side guarantee, not something this module enforces, but it's
// why this file has no notion of "acting as a token" anywhere in it.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  CreateTokenRequest,
  CreateTokenResponse,
  TokenResource,
} from '../types/token'

export const tokenKeys = {
  all: ['tokens'] as const,
  list: () => [...tokenKeys.all, 'list'] as const,
}

async function readErrorMessage(
  res: Response,
  fallback: string,
): Promise<string> {
  const body = (await res.json().catch(() => null)) as { error?: string } | null
  return body?.error ?? fallback
}

// GET /api/v1/auth/tokens (internal/api/tokens.go's handleListTokens).
// Revoked tokens stay in this array, they are not filtered out here:
// TokenTable is responsible for rendering their revoked status distinctly,
// not this fetcher for hiding them.
export async function fetchTokens(): Promise<TokenResource[]> {
  const res = await fetch('/api/v1/auth/tokens')
  if (!res.ok) {
    throw new Error(`fetch tokens failed: ${res.status}`)
  }
  return (await res.json()) as TokenResource[]
}

export function tokenListQueryOptions() {
  return queryOptions({
    queryKey: tokenKeys.list(),
    queryFn: fetchTokens,
  })
}

export function useTokens() {
  return useSuspenseQuery(tokenListQueryOptions())
}

// POST /api/v1/auth/tokens (internal/api/tokens.go's handleCreateToken).
// The response's `token` field is the plaintext credential, present only
// here: CreateTokenDialog is responsible for showing it exactly once and
// never persisting it into any query cache (that's why this mutation
// invalidates the list query on success rather than writing the response
// into it, unlike useUpdateApp's setQueryData pattern in queries/apps.ts:
// caching the plaintext anywhere, even transiently, would work against
// the entire point of "shown once").
export async function createToken(
  req: CreateTokenRequest,
): Promise<CreateTokenResponse> {
  const res = await fetch('/api/v1/auth/tokens', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new Error(
      await readErrorMessage(res, `create token failed: ${res.status}`),
    )
  }
  return (await res.json()) as CreateTokenResponse
}

export function useCreateToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createToken,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: tokenKeys.list() })
    },
  })
}

// DELETE /api/v1/auth/tokens/{id} (internal/api/tokens.go's
// handleRevokeToken). 204 on success, no body to parse.
export async function revokeToken(id: string): Promise<void> {
  const res = await fetch(`/api/v1/auth/tokens/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new Error(
      await readErrorMessage(res, `revoke token failed: ${res.status}`),
    )
  }
}

export function useRevokeToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: revokeToken,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: tokenKeys.list() })
    },
  })
}
