// URL construction for one database's live container-log SSE stream, the
// database-kind counterpart to queries/liveLogs.ts. See that module's own
// header comment for why this isn't a TanStack Query fetcher.
//
// Backend: GET /api/v1/databases/{name}/logs/stream
// (internal/api/database_logs.go), Accept: text/event-stream.
export function buildLiveDatabaseLogStreamUrl(name: string): string {
  const url = new URL(
    `/api/v1/databases/${encodeURIComponent(name)}/logs/stream`,
    window.location.origin,
  )
  return url.toString()
}
