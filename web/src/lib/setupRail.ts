// Pure step-state logic for the setup wizard's progress rail.

import { SETUP_STEPS, nextStep } from './setupWizard'
import type { SetupStepId, SetupStepMap } from './setupWizard'

export type RailState =
  'done' | 'current' | 'attention' | 'skipped' | 'upcoming'

/** railState derives one rail row's state; a completed step with open warnings reads as attention, never as done. */
export function railState(
  id: SetupStepId,
  current: SetupStepId,
  steps: SetupStepMap,
  attention: ReadonlySet<SetupStepId>,
): RailState {
  if (id === current) return 'current'
  const status = steps[id]
  if (status === 'skipped') return 'skipped'
  if (status === 'completed') return attention.has(id) ? 'attention' : 'done'
  return 'upcoming'
}

export interface RailProgress {
  done: number
  total: number
}

/** railProgress counts steps that are settled (completed or skipped) out of all of them. */
export function railProgress(steps: SetupStepMap): RailProgress {
  const settled = SETUP_STEPS.filter((id) => steps[id] !== undefined).length
  return { done: settled, total: SETUP_STEPS.length }
}

export interface NavState {
  current: SetupStepId
}

export type NavAction =
  { type: 'goto'; id: SetupStepId } | { type: 'next' } | { type: 'prev' }

/** setupNavReducer moves the current step, clamped to the first and last step. */
export function setupNavReducer(state: NavState, action: NavAction): NavState {
  switch (action.type) {
    case 'goto':
      return { current: action.id }
    case 'next':
      return { current: nextStep(state.current) }
    case 'prev': {
      const i = SETUP_STEPS.indexOf(state.current)
      return { current: SETUP_STEPS[Math.max(i - 1, 0)] ?? state.current }
    }
  }
}
