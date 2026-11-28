import type { TokenResource } from './token'

// Server-computed in internal/api/tokens_query.go; the dashboard never derives it from dates.
export type TokenStatus = 'active' | 'expiring' | 'expired' | 'revoked'
export type TokenKind = 'emergency' | 'agent' | 'system' | 'user'
export type TokenSort = 'name' | 'created' | 'last_used' | 'expires'

export interface ServedToken extends TokenResource {
  status: Exclude<TokenStatus, 'expiring'>
}

export type StatusCounts = Record<TokenStatus, number>

export interface TokenPage {
  items: ServedToken[]
  total: number
  counts_by_status: StatusCounts
  next_cursor?: string
}

export interface TokenGroup {
  name: string
  count: number
  active: number
  expiring: number
  expired: number
  oldest_created: string
  newest_last_used?: string
}

export interface TokenGroupsPage {
  groups: TokenGroup[]
  total: number
  counts_by_status: StatusCounts
}

// Same shape the list endpoint reads from the query string and bulk revoke reads as `filter`.
export interface TokenFilter {
  q?: string
  status?: TokenStatus
  ability?: string
  name?: string
  kind?: TokenKind
}

export interface BulkRevokeResult {
  dry_run: boolean
  count: number
  sample: string[]
  revoked: number
  max: number
}

export interface PruneResult {
  dry_run: boolean
  revoked: number
  expired: number
}
