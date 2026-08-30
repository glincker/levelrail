// Wire type for an environment, GET/POST /api/v1/projects/{id}/environments,
// DELETE /api/v1/environments/{id} (internal/api/environments.go's
// environmentResource). Scoped to one project: a staging/production-style
// label an app is tagged with via PUT /api/v1/apps/{name}/environment.

export interface EnvironmentResource {
  id: string
  project_id: string
  name: string
  created_at: string
}
