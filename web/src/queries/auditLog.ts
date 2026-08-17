// Fetcher and hook for GET /api/v1/audit-log (internal/api/audit.go):
// every recorded write/deploy/root-tier request, newest first,
// cursor-paginated by `before` (an RFC3339 timestamp) rather than an
// offset, so a large table never gets slower to page through.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AuditLogEntry {
  id: string
  actor_type: string
  actor_id: string
  actor_name: string
  ability: string
  method: string
  path: string
  status_code: number
  remote_addr: string
  created_at: string
}

export const auditLogKeys = {
  all: ['audit-log'] as const,
  list: (before?: string) => [...auditLogKeys.all, 'list', before ?? null] as const,
}

export async function fetchAuditLog(opts: {
  limit?: number
  before?: string
} = {}): Promise<AuditLogEntry[]> {
  const params = new URLSearchParams()
  if (opts.limit) params.set('limit', String(opts.limit))
  if (opts.before) params.set('before', opts.before)
  const qs = params.toString()
  const res = await fetch(`/api/v1/audit-log${qs ? `?${qs}` : ''}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch audit log failed: ${res.status}`),
    )
  }
  return (await res.json()) as AuditLogEntry[]
}

export function auditLogQueryOptions(before?: string) {
  return queryOptions({
    queryKey: auditLogKeys.list(before),
    queryFn: () => fetchAuditLog({ before }),
  })
}

export function useAuditLog(before?: string) {
  return useQuery(auditLogQueryOptions(before))
}
