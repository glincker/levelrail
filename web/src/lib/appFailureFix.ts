import type { AppStatusSummary } from '../types/appDetail'

export type FixTarget = 'deploys' | 'health' | 'domains' | 'pages'

const HEALTH = /probe|readiness|liveness|health/
const DOMAINS = /domain|certificate|tls|dns|acme/

/** Picks the page most likely to fix a failing app from the status condition text. */
export function fixTarget(
  status: Pick<AppStatusSummary, 'reason' | 'message'>,
  isStatic: boolean,
): FixTarget {
  if (isStatic) return 'pages'
  const text = `${status.reason ?? ''} ${status.message ?? ''}`.toLowerCase()
  if (HEALTH.test(text)) return 'health'
  if (DOMAINS.test(text)) return 'domains'
  return 'deploys'
}
