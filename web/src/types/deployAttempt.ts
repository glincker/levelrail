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
// no build step (and so no commit_sha), 'auto_rollback' for an
// unattended rollback triggered by internal/alerting.MaybeAutoRollback
// once a crashloop alert fires (also no commit_sha: it's the same
// image-tag path as 'image', just driven automatically).
export type DeployAttemptSource =
  'webhook' | 'manual' | 'image' | 'auto_rollback'

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
  /** The create-app-from-git wizard's pre-flight detection result for
   *  this build, e.g. "Node.js". Absent when detection was skipped or
   *  found nothing buildable. */
  detected_framework?: string
}
