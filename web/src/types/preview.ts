// Wire types for the deploy preview screenshot routes
// (internal/api/preview.go): GET/PUT /api/v1/apps/{name}/preview,
// POST .../preview/capture and .../preview/prune,
// GET .../preview/history.

export type PreviewRecordStatus = 'ok' | 'skipped' | 'failed'

/** How much a per-app preview may cost: nothing, one small page fetch, or a browser. */
export type PreviewMode = 'off' | 'metadata' | 'screenshot'

/** Where a preview's pixels came from. A card has no image: the UI composes it. */
export type PreviewSource = 'screenshot' | 'og_image' | 'card'

/** Page metadata a card preview is composed from. */
export interface PreviewCardMeta {
  title?: string
  description?: string
  /** Validated hex color such as #112233. */
  theme_color?: string
}

export interface PreviewRecord {
  deployment_id: string
  status: PreviewRecordStatus
  /** Machine code for why a preview is missing, e.g. "auth_wall". */
  reason?: string
  /** Short human sentence with the specifics of reason. */
  detail?: string
  http_status?: number
  path: string
  source: PreviewSource
  meta?: PreviewCardMeta
  width?: number
  height?: number
  bytes: number
  captured_at: string
  /** Set only when status is "ok". */
  image_url?: string
}

export interface PreviewStatus {
  app: string
  /** True for any mode except off. Kept for older clients. */
  enabled: boolean
  mode: PreviewMode
  /** The mode a new app starts in, set by the server. */
  default_mode: PreviewMode
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
  mode?: PreviewMode
  enabled?: boolean
  path?: string
  wait_ms?: number
}

export interface PreviewPruneResult {
  removed: number
  freed_bytes: number
}
