import type { ReconcileCondition } from '../types/deploy'

export type ConvergenceState = 'converged' | 'reconciling' | 'error' | 'unknown'

// deriveConvergence reads the same ReconcileCondition[] ConditionsPanel
// already renders and reduces it to one of four states. 'error' takes
// priority over 'reconciling': a condition that has actively failed is a
// stronger signal than one that simply hasn't reported True yet. 'unknown'
// covers both no conditions at all and every condition sitting at
// Status: 'Unknown', since neither case supports claiming convergence or
// an active failure.
export function deriveConvergence(
  conditions: ReconcileCondition[],
): ConvergenceState {
  if (conditions.length === 0) {
    return 'unknown'
  }
  if (conditions.some((c) => c.Status === 'False')) {
    return 'error'
  }
  if (conditions.every((c) => c.Status === 'True')) {
    return 'converged'
  }
  if (conditions.every((c) => c.Status === 'Unknown')) {
    return 'unknown'
  }
  return 'reconciling'
}
