import type { Deployment } from '../types/deployment'
import {
  canCancel,
  canRedeploy,
  canRollbackTo,
  isInProgress,
  statusView,
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

export function cancelReason(d: Deployment): string {
  if (canCancel(d)) return ''
  if (d.is_live) return 'This deploy is already live, roll back instead'
  if (d.status === 'canceled') return 'This deploy was already canceled'
  return `This deploy already finished (${statusView(d.status).label.toLowerCase()})`
}

export function promoteReason(d: Deployment, hasProject: boolean): string {
  if (!d.is_live) return 'Promote uses the live release of the app'
  return hasProject
    ? ''
    : 'The app is not part of a project with other environments'
}
