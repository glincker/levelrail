import type { AppStatusSummary } from './appDetail'

// Wire type for GET /api/v1/apps/{name}/previews
// (internal/api/preview_environments_handlers.go's
// handleListPreviewEnvironments, previewEnvironmentResource): one active
// preview environment per open pull request against a git-connected,
// preview-enabled app.
export type PreviewEnvironmentStatus = 'deploying' | 'active' | 'failed'

export interface PreviewEnvironment {
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
