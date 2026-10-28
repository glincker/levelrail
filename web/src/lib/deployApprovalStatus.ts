import type { DeployApprovalStatus } from '../types/deployApproval'

export const KNOWN_DEPLOY_APPROVAL_STATUSES: readonly DeployApprovalStatus[] = [
  'pending',
  'approving',
  'approved',
  'rejected',
  'expired',
]

// Only a pending row can be approved or rejected: an approving row is
// claimed by another approver, and decided rows are final.
export function canDecideApproval(status: string): boolean {
  return status === 'pending'
}

export function isKnownDeployApprovalStatus(
  status: string,
): status is DeployApprovalStatus {
  return (KNOWN_DEPLOY_APPROVAL_STATUSES as readonly string[]).includes(status)
}
