// Wire type for an environment, GET/POST /api/v1/projects/{id}/environments,
// DELETE /api/v1/environments/{id} (internal/api/environments.go's
// environmentResource). Scoped to one project: a staging/production-style
// label an app is tagged with via PUT /api/v1/apps/{name}/environment.

export interface EnvironmentResource {
  id: string
  project_id: string
  name: string
  // protected requires confirm: true on a deploy, rollback, or
  // promotion targeting an app tagged with this environment
  // (internal/api's deploys.go/promote.go).
  protected: boolean
  created_at: string
}

// One row of GET /api/v1/environments (internal/api/environments_global.go's
// environmentListResource): global and project environments together.
export type EnvironmentKind =
  'dev' | 'test' | 'uat' | 'production' | 'preview' | 'custom'

export interface GlobalEnvironment extends EnvironmentResource {
  kind: EnvironmentKind
  scope: 'project' | 'global'
  sort_order: number
  app_count: number
  database_count: number
}
