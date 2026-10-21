import type { SelfUpgradeAttempt } from '../queries/selfUpgrade'

export type StepperStepId =
  'check' | 'preflight' | 'download' | 'apply' | 'verify'

export type StepperStatus =
  'done' | 'current' | 'failed' | 'blocked' | 'upcoming'

export const STEPPER_ORDER: StepperStepId[] = [
  'check',
  'preflight',
  'download',
  'apply',
  'verify',
]

const STEP_STAGE: Record<string, StepperStepId> = {
  stop: 'apply',
  swap: 'apply',
  start: 'apply',
  health: 'verify',
}

export interface StepperInput {
  updateAvailable: boolean
  checkFailed: boolean
  preflightBlocked: boolean
  attempt?: SelfUpgradeAttempt
}

export interface StepperStep {
  id: StepperStepId
  status: StepperStatus
}

type Marks = Partial<Record<StepperStepId, StepperStatus>>

function build(marks: Marks): StepperStep[] {
  return STEPPER_ORDER.map((id) => ({ id, status: marks[id] ?? 'upcoming' }))
}

// Download covers everything before the switch (fetch, verify, backup).
function stageOf(step: string): StepperStepId {
  return STEP_STAGE[step] ?? 'download'
}

function fromAttempt(a: SelfUpgradeAttempt): StepperStep[] {
  if (a.outcome === 'succeeded') {
    return build({
      check: 'done',
      preflight: 'done',
      download: 'done',
      apply: 'done',
      verify: 'done',
    })
  }
  if (a.outcome === 'refused')
    return build({ check: 'done', preflight: 'failed' })
  const running = a.outcome === 'running'
  const stage = stageOf(running ? a.current_step : a.failed_step)
  const marks: Marks = { check: 'done', preflight: 'done' }
  for (const id of STEPPER_ORDER.slice(2)) {
    if (id === stage) {
      marks[id] = running ? 'current' : 'failed'
      break
    }
    marks[id] = 'done'
  }
  return build(marks)
}

export function deriveStepper(i: StepperInput): StepperStep[] {
  if (i.attempt) return fromAttempt(i.attempt)
  if (i.checkFailed) return build({ check: 'failed' })
  if (!i.updateAvailable) return build({ check: 'done' })
  if (i.preflightBlocked) return build({ check: 'done', preflight: 'blocked' })
  return build({ check: 'done', preflight: 'done', download: 'current' })
}
