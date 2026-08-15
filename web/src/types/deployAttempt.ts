// Wire type for GET /api/v1/apps/{name}/deploy-attempts
// (internal/api/deploy_attempts.go's handleListDeployAttempts,
// deployAttemptResource). A real, row-per-attempt deploy history,
// additional to types/deploy.ts's ReconcileCondition ("current status",
// not history, see that file's own doc comment for why the two are
// deliberately kept separate rather than one endpoint's shape changing
// underneath its existing consumer).
export type DeployAttemptStatus = 'running' | 'succeeded' | 'failed'

export interface DeployAttempt {
  id: string
  service_name: string
  image: string
  status: DeployAttemptStatus
  started_at: string
  finished_at?: string
  error?: string
}
