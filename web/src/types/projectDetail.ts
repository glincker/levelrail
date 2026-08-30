// Wire type for a project, GET /api/v1/projects, GET/DELETE
// /api/v1/projects/{id}, POST /api/v1/projects (internal/api/
// projects.go's projectResource). Same snake_case-matches-wire-shape
// convention appDetail.ts/nodeDetail.ts document: an internal admin
// dashboard talking to one frozen backend contract, not a public SDK.
//
// A project is a lightweight, non-auth organizational label
// (internal/api/projects.go's own package doc comment): no owner, no
// member list, no permission of any kind. It groups AppDetail/
// DatabaseResource rows via their own project_id field, nothing more.

export interface ProjectResource {
  id: string
  name: string
  created_at: string
  // org_id carries `omitempty` on the Go side: empty means not filed
  // under any organization. Response-only, set via
  // PUT /api/v1/projects/{id}/organization (useSetProjectOrganization).
  org_id?: string
}
