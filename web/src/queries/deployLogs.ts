// URL construction for the deploy log SSE stream. Deliberately not a
// TanStack Query fetcher (see hooks/useLogStream.ts for why a live
// append-only stream does not go through Query's cache): this module's
// only job is building the URL the route hands to the hook, kept in its
// own file for the same reason queries/apps.ts exists, so the URL shape
// is defined once instead of inline in the route component.
//
// Assumed backend contract (not implemented yet as of this pass, see
// internal/api/router.go, and documented again on the hook itself):
//
//   GET /api/v1/apps/{name}/deploys/{deployId}/logs
//   Accept: text/event-stream
//
// Path segments use the app and deploy identifiers as they appear in the
// route params, matching the /api/v1/apps/{name}/... shape the rest of
// the frontend already uses (web/src/routes/apps/$name.tsx). If the real
// backend ends up keying deploys by a different identifier (e.g. a
// numeric deploy sequence vs. a UUID) only this function needs to
// change, not the hook or the route.
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
