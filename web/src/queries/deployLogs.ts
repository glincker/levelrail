// URL construction for the deploy log SSE stream. Deliberately not a
// TanStack Query fetcher (see hooks/useLogStream.ts for why a live
// append-only stream does not go through Query's cache): this module's
// only job is building the URL the route hands to the hook, kept in its
// own file for the same reason queries/apps.ts exists, so the URL shape
// is defined once instead of inline in the route component.
//
// Backend contract (internal/api/router.go):
//
//   GET /api/v1/apps/{name}/deploys/{deployId}/logs
//   Accept: text/event-stream
export function buildDeployLogStreamUrl(
  name: string,
  deployId: string,
): string {
  const url = new URL(
    `/api/v1/apps/${encodeURIComponent(name)}/deploys/${encodeURIComponent(deployId)}/logs`,
    window.location.origin,
  )
  return url.toString()
}
