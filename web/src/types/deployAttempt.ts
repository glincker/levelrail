// Wire type for GET /api/v1/apps/{name}/deploy-attempts
// (internal/api/deploy_attempts.go's handleListDeployAttempts,
// deployAttemptResource). A real, row-per-attempt deploy history,
// additional to types/deploy.ts's ReconcileCondition ("current status",
// not history, see that file's own doc comment for why the two are
// deliberately kept separate rather than one endpoint's shape changing
// underneath its existing consumer).
export type DeployAttemptStatus = 'running' | 'succeeded' | 'failed'

// Mirrors internal/store.DeployAttemptSource* on the wire: 'webhook' for
// an unattended git-push build, 'manual' for a dashboard-triggered
// git-source build, 'image' for a bare image-tag redeploy/rollback with
// no build step (and so no commit_sha).
export type DeployAttemptSource = 'webhook' | 'manual' | 'image'

export interface DeployAttempt {
  id: string
  service_name: string
  image: string
  commit_sha?: string
  source?: DeployAttemptSource
  status: DeployAttemptStatus
  started_at: string
  finished_at?: string
  error?: string
}
