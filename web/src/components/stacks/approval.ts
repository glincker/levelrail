import type { StackRevisionSummary } from '../../types/stacks'

// The server sets the window (APP_STACK_APPROVAL_TTL) but does not send the
// expiry on revisions, so the estimate reads its default from the build env.
const APPROVAL_TTL_HOURS = Number(
  import.meta.env.VITE_STACK_APPROVAL_TTL_HOURS ?? 24,
)

export function approvalExpiry(rev: StackRevisionSummary): Date {
  if (rev.expires_at) return new Date(rev.expires_at)
  return new Date(
    new Date(rev.created_at).getTime() + APPROVAL_TTL_HOURS * 3_600_000,
  )
}

// The server refuses the requester; hiding the button just saves a round trip.
export function canApprove(
  rev: StackRevisionSummary,
  isRoot: boolean,
  currentUser: string | null,
): boolean {
  if (!isRoot || rev.status !== 'pending_approval') return false
  return !currentUser || rev.actor !== currentUser
}
