// Wire types for the deploy preview screenshot routes
// (internal/api/preview.go): GET/PUT /api/v1/apps/{name}/preview,
// POST .../preview/capture and .../preview/prune,
// GET .../preview/history.

export type PreviewRecordStatus = 'ok' | 'skipped' | 'failed'

export interface PreviewRecord {
  deployment_id: string
  status: PreviewRecordStatus
  /** Machine code for why a preview is missing, e.g. "auth_wall". */
  reason?: string
  /** Short human sentence with the specifics of reason. */
  detail?: string
  http_status?: number
  path: string
  width?: number
  height?: number
  bytes: number
  captured_at: string
  /** Set only when status is "ok". */
  image_url?: string
}

export interface PreviewStatus {
  app: string
  enabled: boolean
  path: string
  wait_ms: number
  /** False when the server runs with previews switched off. */
  server_enabled: boolean
  capturing: boolean
  image: string
  browser_image?: { ref: string; last_used_at?: string }
  keep_per_app: number
  ttl_days: number
  max_total_mb: number
  storage: {
    app_bytes: number
    app_count: number
    total_bytes: number
    total_count: number
  }
  latest?: PreviewRecord
}

export interface PreviewSettingsInput {
  enabled?: boolean
  path?: string
  wait_ms?: number
}

export interface PreviewPruneResult {
  removed: number
  freed_bytes: number
}
