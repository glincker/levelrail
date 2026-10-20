// URL of one model's live engine-log SSE stream (internal/api/models.go,
// GET /api/v1/models/{name}/logs/stream).
export function buildLiveModelLogStreamUrl(name: string): string {
  return new URL(
    `/api/v1/models/${encodeURIComponent(name)}/logs/stream`,
    window.location.origin,
  ).toString()
}
