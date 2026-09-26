import type {
  DeploymentActionKind,
  PendingAction,
} from '../hooks/useDeploymentActions'
import { rollbackImage, shortId } from './deploymentPresentation'

export interface ConfirmCopy {
  title: string
  description: string
  confirm: string
  danger: boolean
}

export function confirmCopy(action: PendingAction): ConfirmCopy {
  const d = action.deployment
  const byKind: Record<DeploymentActionKind, ConfirmCopy> = {
    redeploy: {
      title: `Redeploy ${d.app}?`,
      description: `Deploys ${rollbackImage(d)} again to ${d.app} in its current environment. Environment variables and settings stay as they are now.`,
      confirm: 'Redeploy',
      danger: false,
    },
    rollback: {
      title: `Roll back ${d.app} to ${shortId(d.id)}?`,
      description: `Deploys ${rollbackImage(d)} to ${d.app} in its current environment. Only the image changes: environment variables and settings are not reapplied from that release.`,
      confirm: 'Roll back',
      danger: true,
    },
    cancel: {
      title: `Cancel this deploy of ${d.app}?`,
      description:
        'The build stops and the deploy is marked canceled. Whatever is running now keeps running.',
      confirm: 'Cancel deploy',
      danger: true,
    },
  }
  return byKind[action.kind]
}
