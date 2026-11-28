// Filtered, grouped and paged reads of GET /api/v1/auth/tokens plus the bulk
// revoke and prune writes (internal/api/tokens_query.go, tokens_bulk.go).
// Keys sit under tokenKeys.list() so create, rotate and revoke refresh them.

import {
  infiniteQueryOptions,
  keepPreviousData,
  queryOptions,
} from '@tanstack/react-query'
import { useApiMutation } from './useApiMutation'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { TOKEN_PAGE_SIZE } from '../lib/tokenListConfig'
import { tokenKeys } from './tokens'
import type {
  BulkRevokeResult,
  PruneResult,
  TokenFilter,
  TokenGroupsPage,
  TokenPage,
  TokenSort,
} from '../types/tokenList'

function params(
  filter: TokenFilter,
  extra: Record<string, string | undefined> = {},
): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries({ ...filter, ...extra })) {
    if (v) sp.set(k, v)
  }
  return sp.toString()
}

async function getJSON<T>(qs: string): Promise<T> {
  const res = await fetch(`/api/v1/auth/tokens?${qs}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch tokens failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export interface TokenListArgs {
  filter: TokenFilter
  sort: TokenSort
  limit?: number
}

const listKey = (...rest: unknown[]) =>
  [...tokenKeys.list(), 'v2', ...rest] as const

export function tokenPagesQueryOptions({
  filter,
  sort,
  limit = TOKEN_PAGE_SIZE,
}: TokenListArgs) {
  return infiniteQueryOptions({
    queryKey: listKey('page', filter, sort, limit),
    initialPageParam: '',
    queryFn: ({ pageParam }) =>
      getJSON<TokenPage>(
        params(filter, {
          sort,
          limit: String(limit),
          cursor: pageParam || undefined,
        }),
      ),
    getNextPageParam: (last) => last.next_cursor || undefined,
    placeholderData: keepPreviousData,
  })
}

export function tokenGroupsQueryOptions(filter: TokenFilter) {
  return queryOptions({
    queryKey: listKey('groups', filter),
    queryFn: () => getJSON<TokenGroupsPage>(params(filter, { groups: 'name' })),
    placeholderData: keepPreviousData,
  })
}

// One-row read used only for its counts_by_status and total.
export function tokenCountsQueryOptions(filter: TokenFilter) {
  return queryOptions({
    queryKey: listKey('counts', filter),
    queryFn: () => getJSON<TokenPage>(params(filter, { limit: '1' })),
    select: (page) => ({
      counts: page.counts_by_status,
      total: page.total,
    }),
    placeholderData: keepPreviousData,
  })
}

// The most recently used tokens of a filter, for the bulk revoke danger summary.
export function tokenRecentQueryOptions(filter: TokenFilter) {
  return queryOptions({
    queryKey: listKey('recent', filter),
    queryFn: () =>
      getJSON<TokenPage>(params(filter, { sort: 'last_used', limit: '100' })),
    select: (page) => page.items,
  })
}

// Long-lived root tokens: the server has no counter for them, so read the root list once.
export function rootTokensQueryOptions() {
  return queryOptions({
    queryKey: listKey('root-active'),
    queryFn: () =>
      getJSON<TokenPage>(
        params({ ability: 'root', status: 'active' }, { limit: '500' }),
      ),
    select: (page) => page.items.filter((tok) => !tok.expires_at).length,
  })
}

export interface BulkRevokeRequest {
  ids?: string[]
  filter?: TokenFilter
  confirm_count?: number
  dry_run?: boolean
}

export async function bulkRevokeTokens(
  req: BulkRevokeRequest,
): Promise<BulkRevokeResult> {
  const res = await fetch('/api/v1/tokens/bulk-revoke', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `bulk revoke failed: ${res.status}`),
    )
  }
  return (await res.json()) as BulkRevokeResult
}

export function useBulkRevoke() {
  return useApiMutation({
    meta: { errorToast: false },
    fn: bulkRevokeTokens,
    invalidate: (vars) => (vars.dry_run ? [] : [tokenKeys.list()]),
  })
}

export async function pruneTokens(
  olderThanDays: number,
  dryRun: boolean,
): Promise<PruneResult> {
  const res = await fetch('/api/v1/tokens/prune', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ older_than_days: olderThanDays, dry_run: dryRun }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `prune tokens failed: ${res.status}`),
    )
  }
  return (await res.json()) as PruneResult
}

export function usePruneTokens() {
  return useApiMutation({
    meta: { errorToast: false },
    fn: ({ days, dryRun }: { days: number; dryRun: boolean }) =>
      pruneTokens(days, dryRun),
    invalidate: (vars) => (vars.dryRun ? [] : [tokenKeys.list()]),
  })
}
