import { useState } from 'react'
import { toast } from '@/components/ui/toast'
import type { Deployment } from '../types/deployment'
import {
  canCancel,
  canRedeploy,
  canRollbackTo,
  rollbackImage,
} from '../lib/deploymentPresentation'
import { isPendingApproval } from '../queries/deploys'
import {
  isCancelUnsupported,
  useDeploymentMutations,
} from '../queries/deployments'

export type DeploymentActionKind = 'redeploy' | 'rollback' | 'cancel'

export interface PendingAction {
  kind: DeploymentActionKind
  deployment: Deployment
}

export function actionAllowed(
  kind: DeploymentActionKind,
  d: Deployment,
  cancelSupported: boolean,
): boolean {
  if (kind === 'redeploy') return canRedeploy(d)
  if (kind === 'rollback') return canRollbackTo(d)
  return canCancel(d) && cancelSupported
}

export interface DeploymentActions {
  pending: PendingAction | null
  busy: boolean
  cancelSupported: boolean
  request: (kind: DeploymentActionKind, d: Deployment) => void
  confirm: () => void
  dismiss: () => void
}

export function useDeploymentActions(): DeploymentActions {
  const [pending, setPending] = useState<PendingAction | null>(null)
  const [cancelSupported, setCancelSupported] = useState(true)
  const { deployImage, cancel } = useDeploymentMutations()

  const request = (kind: DeploymentActionKind, d: Deployment) => {
    if (actionAllowed(kind, d, cancelSupported))
      setPending({ kind, deployment: d })
  }

  const done = () => {
    setPending(null)
  }

  const confirm = () => {
    if (!pending) return
    const { kind, deployment: d } = pending
    if (kind === 'cancel') {
      cancel.mutate(d, {
        onSuccess: () => {
          toast.add({
            title: `Cancelled the deploy of "${d.app}".`,
            type: 'success',
          })
          done()
        },
        onError: (error) => {
          if (isCancelUnsupported(error)) setCancelSupported(false)
          toast.add({
            title: isCancelUnsupported(error)
              ? 'This server does not support cancelling deploys yet.'
              : error.message,
            type: 'error',
          })
          done()
        },
      })
      return
    }
    const image = rollbackImage(d)
    deployImage.mutate(
      { app: d.app, image },
      {
        onSuccess: (result) => {
          const verb = kind === 'rollback' ? 'Rolling back' : 'Redeploying'
          toast.add({
            title: isPendingApproval(result)
              ? `${verb} "${d.app}" is waiting for approval.`
              : `${verb} "${d.app}".`,
            type: 'success',
          })
          done()
        },
        onError: (error) => {
          toast.add({ title: error.message, type: 'error' })
          done()
        },
      },
    )
  }

  return {
    pending,
    busy: deployImage.isPending || cancel.isPending,
    cancelSupported,
    request,
    confirm,
    dismiss: done,
  }
}
