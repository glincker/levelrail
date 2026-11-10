import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  BulkItemResult,
  BulkItemStatus,
  DbBulkAction,
} from '../lib/databasesFilter'

export interface DatabasesBulkRequest {
  action: DbBulkAction
  names: string[]
  dry_run?: boolean
  confirm_count?: number
}

interface ServerResult {
  name: string
  status: string
  reason?: string
  message?: string
}

interface DatabasesBulkResponse {
  results: ServerResult[]
}

const STATUS: Record<string, BulkItemStatus> = {
  planned: 'planned',
  ok: 'ok',
  skipped: 'skipped',
  denied: 'denied',
  protected: 'protected',
  not_found: 'notFound',
  error: 'error',
}

/** Maps one server result onto the bar's item shape; skips keep their reason code. */
export function toItemResult(r: ServerResult): BulkItemResult {
  const status = STATUS[r.status] ?? 'error'
  return {
    name: r.name,
    status,
    reason: status === 'skipped' ? r.reason : r.message,
  }
}

/** Runs one audited bulk action server side; authz and audit happen per database. */
export async function postDatabasesBulk(
  req: DatabasesBulkRequest,
): Promise<BulkItemResult[]> {
  const res = await fetch('/api/v1/databases/bulk', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `bulk ${req.action} failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as DatabasesBulkResponse
  return body.results.map(toItemResult)
}
