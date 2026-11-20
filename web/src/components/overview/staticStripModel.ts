import type { StatusState } from '@/components/kit'
import type { DeployAttempt } from '../../types/deployAttempt'

export const DEPLOY_STATE: Record<DeployAttempt['status'], StatusState> = {
  running: 'deploying',
  succeeded: 'healthy',
  failed: 'failed',
  held: 'paused',
  awaiting_approval: 'pending',
  superseded: 'stopped',
  queued: 'queued',
  canceled: 'stopped',
  refused: 'failed',
  pending: 'pending',
}

/** Share of 4xx and 5xx responses, or null when nothing was served. */
export function errorShare(
  requests: number,
  status: Record<'2xx' | '3xx' | '4xx' | '5xx', number>,
): number | null {
  if (requests <= 0) return null
  return (status['4xx'] + status['5xx']) / requests
}
