import { Link } from '@tanstack/react-router'
import { HourglassMediumIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useApproveDeployApproval,
  useDeployApprovalsOptional,
  useRejectDeployApproval,
} from '../queries/deployApprovals'
import { ApiError } from '../lib/apiError'

// PendingDeployApprovalBanner surfaces a blocked deploy/promote directly
// on the app it targets, not just in the /approvals queue: the
// app/deploy page is where an operator actually looks when a deploy
// they just triggered doesn't seem to be happening, so the "why" needs
// to be visible right there, not only in a separate cross-app view. One
// app can only ever have one pending request outstanding in practice
// (a second POST from the same environment lands as another row, but
// approving/rejecting the first still leaves later ones visible), so
// this renders the oldest pending request for name, if any.
//
// Approve/Reject act inline: the same-actor rejection
// (loadDecidableApproval, deploy_approvals.go) is enforced server-side,
// so a requester who also happens to hold AbilityDeploy sees the button
// but gets a clear 403 toast rather than the button being hidden (this
// component has no reliable client-side way to know "is the current
// user the same actor who requested this", since a token-authenticated
// dashboard session has no per-request principal identity to compare
// against ahead of time).
export function PendingDeployApprovalBanner({ appName }: { appName: string }) {
  const pending = useDeployApprovalsOptional('pending', appName)
  const approve = useApproveDeployApproval()
  const reject = useRejectDeployApproval()

  const approval = pending.data?.[0]
  if (!approval) {
    return null
  }

  function handleApprove() {
    if (!approval) return
    approve.mutate(approval.id, {
      onSuccess: () => {
        toast.add({
          title: `Approved: ${appName} now targets ${approval.image}.`,
          type: 'success',
        })
      },
      onError: (err) => {
        toast.add({
          title: 'Could not approve.',
          description: err instanceof ApiError ? err.message : String(err),
          type: 'error',
        })
      },
    })
  }

  function handleReject() {
    if (!approval) return
    reject.mutate(
      { id: approval.id },
      {
        onSuccess: () => {
          toast.add({ title: 'Deploy approval rejected.', type: 'success' })
        },
        onError: (err) => {
          toast.add({
            title: 'Could not reject.',
            description: err instanceof ApiError ? err.message : String(err),
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-amber-900 sm:flex-row sm:items-center sm:justify-between dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
      <div className="flex items-start gap-2">
        <HourglassMediumIcon
          className="mt-0.5 size-4 shrink-0"
          aria-hidden="true"
        />
        <div className="text-sm">
          <p className="font-medium">
            {approval.action === 'promote' ? 'Promotion' : 'Deploy'} to{' '}
            <span className="font-mono">{approval.image}</span> is awaiting
            approval.
          </p>
          <p className="text-amber-800/90 dark:text-amber-200/80">
            Requested by {approval.requested_by_name}. This environment is
            protected: a different, sufficiently privileged user must approve it
            before it deploys.
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={approve.isPending || reject.isPending}
          onClick={handleReject}
        >
          Reject
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={approve.isPending || reject.isPending}
          onClick={handleApprove}
        >
          {approve.isPending ? 'Approving...' : 'Approve'}
        </Button>
        <Link
          to="/approvals"
          className="text-xs text-amber-800 underline underline-offset-2 dark:text-amber-200"
        >
          View queue
        </Link>
      </div>
    </div>
  )
}
