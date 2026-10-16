import type { DeployStepEvent } from '../hooks/useDeployStepStream'

// The coarse steps a build deploy emits before the reconciler's own trace.
const BUILD_STEPS = new Set(['detecting', 'building', 'pushing', 'deploying'])

// isTraceStep tells a pipeline trace step from a legacy build step.
export function isTraceStep(ev: DeployStepEvent): boolean {
  return !BUILD_STEPS.has(ev.step) && ev.step !== 'failed'
}
