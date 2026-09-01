// Fetcher and hook for GET /api/v1/audit-log (internal/api/audit.go):
// every recorded write/deploy/root-tier request, newest first,
// cursor-paginated by `before` (an RFC3339 timestamp) rather than an
// offset, so a large table never gets slower to page through.

import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
  client_kind: string
}

export const auditLogKeys = {
  all: ['audit-log'] as const,
  list: (before?: string) => [...auditLogKeys.all, 'list', before ?? null] as const,
  scoped: (path: string, method: string) =>
    [...auditLogKeys.all, 'scoped', path, method] as const,
}

export interface AuditLogQueryOptions {
  limit?: number
  before?: string
  path?: string
  method?: string
  clientKind?: string
}

// buildAuditLogParams is the one place that turns AuditLogQueryOptions
// into GET /api/v1/audit-log's query string, shared by fetchAuditLog and
// auditLogExportURL so the CSV export can't drift from the JSON list's
// own filter params.
function buildAuditLogParams(opts: AuditLogQueryOptions): URLSearchParams {
  const params = new URLSearchParams()
  if (opts.limit) params.set('limit', String(opts.limit))
  if (opts.before) params.set('before', opts.before)
  if (opts.path) params.set('path', opts.path)
  if (opts.method) params.set('method', opts.method)
  if (opts.clientKind) params.set('client_kind', opts.clientKind)
  return params
}

// CLIENT_KIND_OPTIONS mirrors internal/api's ClientKindCLI/Dashboard/MCP/API
// constants: the caller surfaces the audit log's Client column and filter
// can show, in the same order the CLI's own --client-kind flag documents.
export const CLIENT_KIND_OPTIONS = ['cli', 'dashboard', 'mcp', 'api'] as const

export async function fetchAuditLog(
  opts: AuditLogQueryOptions = {},
): Promise<AuditLogEntry[]> {
  const qs = buildAuditLogParams(opts).toString()
  const res = await fetch(`/api/v1/audit-log${qs ? `?${qs}` : ''}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch audit log failed: ${res.status}`),
    )
  }
  return (await res.json()) as AuditLogEntry[]
}

// AUDIT_LOG_EXPORT_LIMIT mirrors internal/api/audit.go's own
// maxAuditLogLimit: the export always asks for the server's largest
// allowed page rather than whatever page size the live table happens to
// be showing, since an export is meant to be as complete as one request
// can make it.
export const AUDIT_LOG_EXPORT_LIMIT = 200

// auditLogExportURL builds GET /api/v1/audit-log?format=csv's URL for
// the current filter state (path/method), reusing buildAuditLogParams
// rather than re-deriving the query string. Consumed as a plain <a href
// download> browser navigation, not a fetch: the response is a raw CSV
// file, not JSON, the same reasoning backupDownloadURL's own doc comment
// gives for the backup download link, and auth rides along on the same
// httpOnly session cookie.
export function auditLogExportURL(
  opts: Omit<AuditLogQueryOptions, 'limit' | 'before'> = {},
): string {
  const params = buildAuditLogParams({ ...opts, limit: AUDIT_LOG_EXPORT_LIMIT })
  params.set('format', 'csv')
  return `/api/v1/audit-log?${params.toString()}`
}

export function auditLogQueryOptions(before?: string, clientKind?: string) {
  return queryOptions({
    queryKey: [...auditLogKeys.list(before), clientKind ?? null] as const,
    queryFn: () => fetchAuditLog({ before, clientKind }),
  })
}

export function useAuditLog(before?: string, clientKind?: string) {
  return useQuery(auditLogQueryOptions(before, clientKind))
}

// Scoped to one resource's own config-change trail (internal/api/
// audit.go's ?path/?method filter): EnvEditor's recent-activity panel
// uses this for an app's PUT /api/v1/apps/{name} history. AbilityRoot
// gated same as the unscoped list, so a non-root actor sees this panel
// fail quietly (see EnvActivityPanel) rather than blocking the editor.
export function appConfigActivityQueryOptions(appName: string, limit = 5) {
  const path = `/api/v1/apps/${appName}`
  return queryOptions({
    queryKey: auditLogKeys.scoped(path, 'PUT'),
    queryFn: () => fetchAuditLog({ path, method: 'PUT', limit }),
  })
}

export function useAppConfigActivity(appName: string, limit = 5) {
  return useQuery({
    ...appConfigActivityQueryOptions(appName, limit),
    retry: false,
  })
}

export interface PurgeAuditLogResult {
  deleted: number
}

// purgeAuditLog calls POST /api/v1/audit-log/purge (internal/api/
// audit_retention.go): deletes every entry older than the control
// plane's own configured retention window right now, for an operator who
// wants old entries cleared immediately rather than waiting for the next
// automatic sweep.
export async function purgeAuditLog(): Promise<PurgeAuditLogResult> {
  const res = await fetch('/api/v1/audit-log/purge', { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `purge audit log failed: ${res.status}`),
    )
  }
  return (await res.json()) as PurgeAuditLogResult
}

export function usePurgeAuditLog() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: purgeAuditLog,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: auditLogKeys.all })
    },
  })
}
