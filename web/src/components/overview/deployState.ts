import type { DeployAttempt } from '../../types/deployAttempt'
import { deployStateKey } from '../pages/overviewHelpers'

/** Dot colour per deploy status, shared by the summary strip and the deployments table. */
export const DEPLOY_DOT: Record<string, string> = {
  succeeded: 'bg-success',
  running: 'bg-warning motion-safe:animate-pulse',
  failed: 'bg-destructive',
}

export function dotClass(status: DeployAttempt['status']): string {
  return DEPLOY_DOT[status] ?? 'bg-muted-foreground/50'
}

/** i18n key suffix for a status, so an unknown server value never renders raw. */
export function stateKey(status: DeployAttempt['status'] | undefined) {
  return deployStateKey(status)
}
