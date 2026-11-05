import type { DeployAttemptStatus } from '../../types/deployAttempt'

export type HeroPrimary = 'deploy' | 'restart' | 'retry'

/** The one primary action for the header: retry a failed deploy, restart a live app, else deploy. */
export function pickHeroPrimary(input: {
  suspended: boolean
  latestStatus?: DeployAttemptStatus
}): HeroPrimary {
  if (input.latestStatus === 'failed') return 'retry'
  if (input.suspended || input.latestStatus === undefined) return 'deploy'
  return 'restart'
}
