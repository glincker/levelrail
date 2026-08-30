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
}
