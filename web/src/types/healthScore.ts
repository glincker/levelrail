// Wire types for GET /api/v1/apps/{name}/health-score
// (internal/api/app_health_score.go's own appHealthScoreResource and
// healthScoreCategory), matching the server's shape exactly: same field
// names, same snake_case JSON tags, same three-state status string
// rather than a boolean or numeric score, because a one-line reason
// per category is the whole point of this endpoint.

export type HealthScoreStatus = 'pass' | 'warn' | 'fail'

export interface AppHealthScoreCategory {
  key: string
  label: string
  status: HealthScoreStatus
  reason: string
}

export interface AppHealthScore {
  app_name: string
  status: HealthScoreStatus
  categories: AppHealthScoreCategory[]
  computed_at: string
}
