import type { Deployment } from '../types/deployment'
import {
  canCancel,
  canRedeploy,
  canRollbackTo,
  isInProgress,
} from './deploymentPresentation'

export function redeployReason(d: Deployment): string {
  if (canRedeploy(d)) return ''
  return isInProgress(d)
    ? 'This deploy is still in progress'
    : 'No image to redeploy'
}

export function rollbackReason(d: Deployment): string {
  if (canRollbackTo(d)) return ''
  if (d.is_live) return 'This is already the live release'
  return 'Only a release that finished ready can be rolled back to'
}

export function cancelReason(d: Deployment, supported: boolean): string {
  if (!canCancel(d)) return 'Only queued or building deploys can be cancelled'
  return supported ? '' : 'This server does not support cancelling deploys yet'
}

export function promoteReason(d: Deployment, hasProject: boolean): string {
  if (!d.is_live) return 'Promote uses the live release of the app'
  return hasProject
    ? ''
    : 'The app is not part of a project with other environments'
}
