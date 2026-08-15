// URL construction for one app's live container-log SSE stream. The
// same "not a TanStack Query fetcher" reasoning queries/deployLogs.ts
// documents applies here unchanged: a live, append-only feed doesn't fit
// Query's cache/freshness model, so this module's only job is building
// the URL the route hands to hooks/useLogStream.ts, kept in its own file
// so the URL shape is defined once instead of inline in the component.
//
// Backend: GET /api/v1/apps/{name}/logs/stream (internal/api/
// live_logs.go), Accept: text/event-stream. Distinct from
// queries/logs.ts, which queries the historical, already-persisted
// search endpoint (GET /api/v1/apps/{name}/logs) instead of tailing
// live.
export function buildLiveLogStreamUrl(name: string): string {
  const url = new URL(
    `/api/v1/apps/${encodeURIComponent(name)}/logs/stream`,
    window.location.origin,
  )
  return url.toString()
}
