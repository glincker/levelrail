// Wire types for GET /api/v1/apps/{name}/health-explain
// (internal/api/app_health_explain.go).

export type HealthExplainState = 'healthy' | 'degraded' | 'failing'

export type HealthExplainNextKind =
  'apply_preset' | 'rollback' | 'view_logs' | 'none'

export interface HealthExplainCheck {
  target: string
  error?: string
  status_code?: number
  since: string
  last_probed: string
}

export interface HealthExplainDeploy {
  id: string
  image: string
  status: string
  started_at: string
  error?: string
}

export interface AppHealthExplain {
  app_name: string
  state: HealthExplainState
  failing_check?: HealthExplainCheck
  restarts?: number
  crashloop: boolean
  last_deploy?: HealthExplainDeploy
  log_error?: string
  next_step: { kind: HealthExplainNextKind; preset?: string; command?: string }
  computed_at: string
}
