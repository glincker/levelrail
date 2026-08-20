// Wire type for an organization, GET /api/v1/organizations, GET/DELETE
// /api/v1/organizations/{id}, POST /api/v1/organizations
// (internal/api/organizations.go's organizationResource).
//
// An organization groups projects, nothing more: no member list, no
// per-organization ability, the same non-auth "purely a label" role
// ProjectResource already documents for itself.

export interface OrganizationResource {
  id: string
  name: string
  created_at: string
}
