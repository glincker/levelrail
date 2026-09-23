// URL construction for the deploy step SSE stream, the same
// "just build the URL, no Query cache" role queries/deployLogs.ts plays
// for the raw log stream.
//
// Backend contract (internal/api/deploy_steps.go):
//
//   GET /api/v1/apps/{name}/deploys/{deployId}/steps
//   Accept: text/event-stream
export function buildDeployStepStreamUrl(
  name: string,
  deployId: string,
): string {
  const url = new URL(
    `/api/v1/apps/${encodeURIComponent(name)}/deploys/${encodeURIComponent(deployId)}/steps`,
    window.location.origin,
  )
  return url.toString()
}
