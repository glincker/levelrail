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

// GET /api/v1/apps/{name}/deploys/{deployId}/logs/download
// (internal/api/deploy_log_download.go), the same attempt's full log as
// a plain-text attachment instead of an SSE stream. Not a TanStack
// Query fetcher for the same reason logDownloadURL (queries/logs.ts)
// isn't: the response is a raw file stream, consumed as a plain
// browser navigation target (an <a href download>), auth riding along
// on the same httpOnly session cookie every other same-origin request
// already relies on.
export function deployLogDownloadURL(name: string, deployId: string): string {
  return `/api/v1/apps/${encodeURIComponent(name)}/deploys/${encodeURIComponent(deployId)}/logs/download`
}
