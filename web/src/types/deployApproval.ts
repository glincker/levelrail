// Wire types for internal/api's deploy_approvals.go: the two-person
// approval gate a deploy/promote into a protected environment now goes
// through, replacing the old same-actor confirm-flag gate. Field names
// mirror the JSON wire shape directly, the same convention
// types/appDetail.ts's own doc comment establishes for this dashboard.

export type DeployApprovalStatus =
  'pending' | 'approved' | 'rejected' | 'expired'

export type DeployApprovalAction = 'deploy' | 'promote'

export interface DeployApprovalResource {
  id: string
  service_name: string
  source_service_name?: string
  environment_id: string
  action: DeployApprovalAction
  image: string
  status: DeployApprovalStatus
  requested_by_type: string
  requested_by: string
  requested_by_name: string
  approved_by_type?: string
  approved_by?: string
  approved_by_name?: string
  reason?: string
  created_at: string
  expires_at: string
  decided_at?: string
}

export interface DeployApprovalListResult {
  approvals: DeployApprovalResource[]
}

export interface DeployApprovalDecisionResult {
  approval: DeployApprovalResource
  app: { name: string; image: string }
}
