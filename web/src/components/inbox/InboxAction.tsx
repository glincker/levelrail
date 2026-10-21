import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import type { InboxAction as Action } from '../../lib/inboxEntries'
import {
  useApproveDeployApproval,
  useRejectDeployApproval,
} from '../../queries/deployApprovals'
import { useTriggerDeploy } from '../../queries/deploys'
import { SignInDecision } from './SignInDecision'

export function InboxAction({
  action,
  onDone,
}: {
  action: Action
  onDone: () => void
}) {
  switch (action.type) {
    case 'deployApproval':
      return <ApprovalAction id={action.approvalId} onDone={onDone} />
    case 'signIn':
      return <SignInDecision id={action.approvalId} onDone={onDone} />
    case 'retryDeploy':
      return (
        <RetryAction app={action.app} image={action.image} onDone={onDone} />
      )
    case 'link':
      return <LinkAction to={action.to} />
  }
}

function LinkAction({ to }: { to: string }) {
  const { t } = useTranslation('inbox')
  return (
    <Button size="sm" render={<Link to={to} />} nativeButton={false}>
      {t('page.fix')}
    </Button>
  )
}

function ApprovalAction({ id, onDone }: { id: string; onDone: () => void }) {
  const { t } = useTranslation('inbox')
  const approve = useApproveDeployApproval()
  const reject = useRejectDeployApproval()
  const busy = approve.isPending || reject.isPending
  const fail = () => {
    toast.add({ title: t('page.actionError'), type: 'error' })
  }
  return (
    <div className="flex items-center gap-2">
      <Button
        size="sm"
        disabled={busy}
        onClick={() => {
          approve.mutate(id, { onSuccess: onDone, onError: fail })
        }}
      >
        {t('page.approve')}
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={busy}
        onClick={() => {
          reject.mutate({ id }, { onSuccess: onDone, onError: fail })
        }}
      >
        {t('page.deny')}
      </Button>
    </div>
  )
}

function RetryAction({
  app,
  image,
  onDone,
}: {
  app: string
  image: string
  onDone: () => void
}) {
  const { t } = useTranslation('inbox')
  const trigger = useTriggerDeploy(app)
  return (
    <Button
      size="sm"
      disabled={trigger.isPending}
      onClick={() => {
        trigger.mutate(
          { image },
          {
            onSuccess: () => {
              toast.add({
                title: t('page.retryStarted', { app }),
                type: 'success',
              })
              onDone()
            },
            onError: () => {
              toast.add({ title: t('page.actionError'), type: 'error' })
            },
          },
        )
      }}
    >
      {t('page.retry')}
    </Button>
  )
}
