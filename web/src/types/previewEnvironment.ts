import type { AppStatusSummary } from './appDetail'

// Wire type for GET /api/v1/apps/{name}/previews
// (internal/api/preview_environments_handlers.go's
// handleListPreviewEnvironments, previewEnvironmentResource): one active
// preview environment per open pull request against a git-connected,
// preview-enabled app.
export type PreviewEnvironmentStatus =
  'deploying' | 'active' | 'failed' | 'awaiting_approval' | 'limit_reached'

export interface PreviewEnvironment {
  app_name: string
  pr_number: number
  preview_app_id: string
  branch: string
  head_sha: string
  domain?: string
  status: PreviewEnvironmentStatus
  status_reason?: string
  created_at: string
  updated_at: string
  stale: boolean
  expires_at?: string
  is_fork: boolean
  head_repo?: string
  ephemeral_databases?: PreviewEphemeralDatabase[]
}

// PreviewEphemeralDatabase mirrors internal/api's
// previewEphemeralDatabaseResource: one disposable, preview-scoped
// database instance provisioned for a databases: entry with
// ephemeralInPreviews set (internal/spec.Database.EphemeralInPreviews).
// `status` only ever tracks this row's own bookkeeping
// (provisioned/teardown_failed); `ready` is the underlying container's
// live reconcile status, the same shape GET /api/v1/databases already
// returns per database.
export interface PreviewEphemeralDatabase {
  source_key: string
  database_name: string
  engine: string
  version: string
  status: 'provisioned' | 'teardown_failed'
  status_reason?: string
  ready: AppStatusSummary
  created_at: string
  updated_at: string
}

export type PreviewOnLimit = 'evict_oldest' | 'reject'

// GET/PUT /api/v1/apps/{name}/preview-policy (internal/api's
// previewPolicyResource): the app's preview policy plus live usage against
// the platform caps. A cap of 0 means unlimited.
export interface PreviewPolicy {
  on_limit: PreviewOnLimit
  allow_fork_previews: boolean
  ttl_hours: number
  effective_ttl_hours: number
  max_per_app: number
  live_count: number
  max_total: number
  live_total: number
}

export interface PreviewPolicyUpdate {
  on_limit?: PreviewOnLimit
  allow_fork_previews?: boolean
  ttl_hours?: number
}
