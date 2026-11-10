import { Link } from '@tanstack/react-router'
import { CountdownGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useApproveDeployApproval,
  useDeployApprovalsOptional,
  useRejectDeployApproval,
} from '../queries/deployApprovals'
import { ApiError } from '../lib/apiError'
import { digestOfImage, unpinnedImage } from '../lib/imageDigest'
import { PinnedDigestBadge } from './PinnedDigestBadge'
import { canDecideApproval } from '../lib/deployApprovalStatus'

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
  const { t } = useTranslation('deploys')
  const pending = useDeployApprovalsOptional('pending', appName)
  const approve = useApproveDeployApproval()
  const reject = useRejectDeployApproval()

  const approval = pending.data?.[0]
  const digest = approval ? digestOfImage(approval.image) : ''
  if (!approval || !canDecideApproval(approval.status)) {
    return null
  }

  function handleApprove() {
    if (!approval) return
    approve.mutate(approval.id, {
      onSuccess: (res) => {
        if (res.approval.status === 'pending') {
          toast.add({
            title: t('approvalBanner.recorded', {
              received: res.approval.approvals_received ?? 0,
              required: res.approval.approvals_required ?? 0,
            }),
            type: 'success',
          })
          return
        }
        toast.add({
          title: t('approvalBanner.approved', {
            app: appName,
            image: approval.image,
          }),
          type: 'success',
        })
      },
      onError: (err) => {
        toast.add({
          title: t('approvalBanner.approveError'),
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
          toast.add({
            title: t('approvalBanner.rejected'),
            type: 'success',
          })
        },
        onError: (err) => {
          toast.add({
            title: t('approvalBanner.rejectError'),
            description: err instanceof ApiError ? err.message : String(err),
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Alert
      variant="warning"
      role="status"
      className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between"
    >
      <div className="flex items-start gap-2">
        <CountdownGlassIcon
          className="mt-0.5 size-5 shrink-0"
          aria-hidden="true"
        />
        <div className="text-sm">
          <p className="font-medium">
            {t(
              approval.action === 'promote'
                ? 'approvalBanner.titlePromote'
                : 'approvalBanner.titleDeploy',
            )}{' '}
            <span className="break-all font-mono">
              {unpinnedImage(approval.image)}
            </span>
            {digest ? (
              <PinnedDigestBadge digest={digest} className="mx-1" />
            ) : null}{' '}
            {t('approvalBanner.awaiting')}
          </p>
          <p className="text-muted-foreground">
            {t('approvalBanner.requestedBy', {
              name: approval.requested_by_name,
            })}
          </p>
          {(approval.approvals_required ?? 0) > 1 ? (
            <p className="text-muted-foreground">
              {t('approvalBanner.progress', {
                received: approval.approvals_received ?? 0,
                required: approval.approvals_required,
              })}
            </p>
          ) : null}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={approve.isPending || reject.isPending}
          onClick={handleReject}
        >
          {t('approvalBanner.reject')}
        </Button>
        <Button
          type="button"
          disabled={approve.isPending || reject.isPending}
          onClick={handleApprove}
        >
          {approve.isPending
            ? t('approvalBanner.approving')
            : t('approvalBanner.approve')}
        </Button>
        <Link
          to="/approvals"
          className="inline-flex min-h-10 items-center text-xs underline underline-offset-2"
        >
          {t('approvalBanner.viewQueue')}
        </Link>
      </div>
    </Alert>
  )
}
