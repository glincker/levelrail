// Types and pure helpers for the Get started hub (internal/setupguide).

export type SetupGuideStepId =
  | 'secure_admin'
  | 'public_address'
  | 'git_provider'
  | 'first_app'
  | 'app_domain'
  | 'dns'
  | 'tls'
  | 'backup_target'
  | 'first_backup'
  | 'verified_restore'
  | 'notifications'
  | 'teammate'
  | 'upgrades'

export type SetupGuideState =
  'todo' | 'in_progress' | 'done' | 'skipped' | 'not_applicable'

export type SetupGuideReason =
  | 'tls_terminated_upstream'
  | 'proxy_mode'
  | 'unavailable'
  | 'http_only'
  | 'no_alert_rule'
  | 'certificates_pending'
  | 'acme_disabled'
  | 'marked_done'

export interface SetupGuideStep {
  id: SetupGuideStepId
  title: string
  state: SetupGuideState
  optional: boolean
  estimate_minutes: number
  href: string
  blocked_by?: SetupGuideStepId[]
  reason?: SetupGuideReason
  progress?: { done: number; total: number }
  manual?: boolean
}

export interface SetupGuide {
  steps: SetupGuideStep[]
  ready_percent: number
  done: number
  total: number
  next_step?: SetupGuideStepId
  complete: boolean
  hidden: boolean
  server_address?: string
}

export type SetupGuideOverride = 'skipped' | 'done' | 'todo'

/** isOpen is true while a step still wants the operator's attention. */
export function isOpen(step: SetupGuideStep): boolean {
  return step.state === 'todo' || step.state === 'in_progress'
}

/** guideStep finds one step by id. */
export function guideStep(
  guide: SetupGuide | undefined,
  id: SetupGuideStepId,
): SetupGuideStep | undefined {
  return guide?.steps.find((s) => s.id === id)
}

/** showGuideEntryPoints decides whether the sidebar item, chip and hints appear. */
export function showGuideEntryPoints(guide: SetupGuide | undefined): boolean {
  return guide !== undefined && !guide.hidden && !guide.complete
}

/** stepGroups splits the list into the focus step, the rest to do, and settled steps. */
export function stepGroups(guide: SetupGuide): {
  next: SetupGuideStep | undefined
  todo: SetupGuideStep[]
  settled: SetupGuideStep[]
} {
  const next = guide.steps.find((s) => s.id === guide.next_step)
  const todo = guide.steps.filter((s) => isOpen(s) && s.id !== next?.id)
  const settled = guide.steps.filter((s) => !isOpen(s))
  return { next, todo, settled }
}

/** ESSENTIAL_STEPS is the "Ship your first app" group: the deploy comes before any server hardening. */
export const ESSENTIAL_STEPS: readonly SetupGuideStepId[] = ['first_app']

/** PRODUCTION_FIRST orders the "Make it production ready" group; unlisted steps keep the server order after these. */
const PRODUCTION_FIRST: readonly SetupGuideStepId[] = [
  'public_address',
  'secure_admin',
  'git_provider',
]

/** essentialSplit separates the ship-your-first-app steps from the production-ready ones; every step lands in exactly one list. */
export function essentialSplit(guide: SetupGuide): {
  essential: SetupGuideStep[]
  more: SetupGuideStep[]
} {
  const essential = ESSENTIAL_STEPS.flatMap((id) => {
    const step = guideStep(guide, id)
    return step ? [step] : []
  })
  const rank = (s: SetupGuideStep) => {
    const at = PRODUCTION_FIRST.indexOf(s.id)
    return at === -1 ? PRODUCTION_FIRST.length : at
  }
  const more = guide.steps
    .filter((s) => !ESSENTIAL_STEPS.includes(s.id))
    .sort((a, b) => rank(a) - rank(b))
  return { essential, more }
}
