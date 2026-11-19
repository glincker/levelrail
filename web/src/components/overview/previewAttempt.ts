import type { DeployAttempt } from '../../types/deployAttempt'

/** Serving release first, then any succeeded one: the page that is live now. */
export function pickPreviewAttempt(
  attempts: DeployAttempt[],
): DeployAttempt | undefined {
  return (
    attempts.find(
      (a) => a.status === 'succeeded' && a.rollout_state === 'serving',
    ) ?? attempts.find((a) => a.status === 'succeeded')
  )
}
