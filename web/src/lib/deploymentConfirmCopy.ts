import type {
  DeploymentActionKind,
  PendingAction,
} from '../hooks/useDeploymentActions'
import { pinnedRef, rollbackImage, shortId } from './deploymentPresentation'

export interface ConfirmCopy {
  title: string
  description: string
  confirm: string
  danger: boolean
}

export function confirmCopy(action: PendingAction): ConfirmCopy {
  const d = action.deployment
  const where = d.environment ? ` (${d.environment})` : ''
  const byKind: Record<DeploymentActionKind, ConfirmCopy> = {
    redeploy: {
      title: `Redeploy ${d.app}?`,
      description: `Deploys ${d.image} again to ${d.app}${where}. Environment variables and settings stay as they are now.`,
      confirm: 'Redeploy',
      danger: false,
    },
    rollback: {
      title: `Roll back ${d.app} to ${shortId(d.id)}?`,
      description: `Deploys ${pinnedRef(d) || rollbackImage(d)} to ${d.app}${where}. Only the image changes: environment variables and settings are not reapplied from that release.`,
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
