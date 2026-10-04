import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { toast, toastAction } from '@/components/ui/toast'
import type { Deployment } from '../types/deployment'
import {
  canCancel,
  canRedeploy,
  canRollbackTo,
  rollbackImage,
} from '../lib/deploymentPresentation'
import { isPendingApproval } from '../queries/deploys'
import { useDeploymentMutations } from '../queries/deployments'

export type DeploymentActionKind = 'redeploy' | 'rollback' | 'cancel'

export interface PendingAction {
  kind: DeploymentActionKind
  deployment: Deployment
}

export function actionAllowed(
  kind: DeploymentActionKind,
  d: Deployment,
): boolean {
  if (kind === 'redeploy') return canRedeploy(d)
  if (kind === 'rollback') return canRollbackTo(d)
  return canCancel(d)
}

export interface DeploymentActions {
  pending: PendingAction | null
  busy: boolean
  request: (kind: DeploymentActionKind, d: Deployment) => void
  confirm: () => void
  dismiss: () => void
}

export function useDeploymentActions(
  onViewApproval?: () => void,
): DeploymentActions {
  const { t } = useTranslation('common')
  const navigate = useNavigate()
  const [pending, setPending] = useState<PendingAction | null>(null)
  const { deployImage, cancel, rollback } = useDeploymentMutations()

  const viewAppAction = (appName: string) =>
    toastAction(t('actions.viewApp'), () => {
      void navigate({ to: '/apps/$name/overview', params: { name: appName } })
    })

  const request = (kind: DeploymentActionKind, d: Deployment) => {
    if (actionAllowed(kind, d)) setPending({ kind, deployment: d })
  }

  const done = () => {
    setPending(null)
  }

  const failed = (error: Error) => {
    toast.add({ title: error.message, type: 'error' })
    done()
  }

  const confirm = () => {
    if (!pending) return
    const { kind, deployment: d } = pending
    if (kind === 'cancel') {
      cancel.mutate(d, {
        onSuccess: () => {
          toast.add({
            title: `Canceled the deploy of "${d.app}".`,
            type: 'success',
          })
          done()
        },
        onError: failed,
      })
      return
    }
    if (kind === 'rollback') {
      rollback.mutate(d, {
        onSuccess: (result) => {
          if (isPendingApproval(result)) {
            toast.add({
              title: `Rolling back "${d.app}" is waiting for approval.`,
              type: 'warning',
              actionProps: onViewApproval
                ? { children: 'View approval', onClick: onViewApproval }
                : undefined,
            })
          } else {
            toast.add({
              title: `Rolling back "${d.app}".`,
              type: 'success',
              actionProps: viewAppAction(d.app),
            })
          }
          done()
        },
        onError: failed,
      })
      return
    }
    deployImage.mutate(
      { app: d.app, image: rollbackImage(d) },
      {
        onSuccess: (result) => {
          const awaitingApproval = isPendingApproval(result)
          toast.add({
            title: awaitingApproval
              ? `Redeploying "${d.app}" is waiting for approval.`
              : `Redeploying "${d.app}".`,
            type: 'success',
            actionProps: awaitingApproval ? undefined : viewAppAction(d.app),
          })
          done()
        },
        onError: failed,
      },
    )
  }

  return {
    pending,
    busy: deployImage.isPending || cancel.isPending || rollback.isPending,
    request,
    confirm,
    dismiss: done,
  }
}
