import type { SelfUpgradeAttempt } from '../queries/selfUpgrade'
import type { RowState } from '../components/settings/upgradeProgressRows'

export type PhaseId =
  'prepare' | 'validate' | 'protect' | 'switch' | 'confirm' | 'other'

export const PHASE_ORDER: PhaseId[] = [
  'prepare',
  'validate',
  'protect',
  'switch',
  'confirm',
]

const STEP_PHASE: Record<string, PhaseId> = {
  guard: 'prepare',
  download: 'prepare',
  verify_checksum: 'prepare',
  verify_signature: 'prepare',
  probe_binary: 'validate',
  migration_check: 'validate',
  backup: 'protect',
  stop: 'switch',
  swap: 'switch',
  start: 'switch',
  health: 'confirm',
}

// Steps the plan may omit still render in the bar and rail.
export const DEFAULT_STEPS = Object.keys(STEP_PHASE).filter(
  (s) => s !== 'guard',
)

export interface StepView {
  name: string
  state: RowState
  detail: string
  durationMs: number
  startedAt: number | null
}

export interface PhaseView {
  id: PhaseId
  state: RowState
  steps: StepView[]
  durationMs: number
}

export interface PipelineView {
  phases: PhaseView[]
  percent: number | null
  stepIndex: number
  total: number
  currentStep: string
  currentPhase: PhaseId | null
  rolledBack: boolean
}

const ROLLBACK = 'rollback'

// An unknown step joins the phase of the step before it, else "other".
export function phaseOf(name: string, prev: PhaseId | null): PhaseId {
  return STEP_PHASE[name] ?? prev ?? 'other'
}

function phaseState(steps: StepView[]): RowState {
  if (steps.some((s) => s.state === 'failed')) return 'failed'
  if (steps.some((s) => s.state === 'running')) return 'running'
  const settled = steps.filter((s) => s.state === 'ok' || s.state === 'skipped')
  if (settled.length === 0) return 'pending'
  if (settled.length < steps.length) return 'running'
  return settled.every((s) => s.state === 'skipped') ? 'skipped' : 'ok'
}

export function buildPipeline(
  planned: string[],
  attempt: SelfUpgradeAttempt | undefined,
): PipelineView {
  const names = [
    'guard',
    ...(planned.length ? planned : DEFAULT_STEPS).filter((n) => n !== 'guard'),
  ]
  const done = new Map((attempt?.steps ?? []).map((s) => [s.name, s]))
  if (done.has(ROLLBACK) && !names.includes(ROLLBACK)) names.push(ROLLBACK)
  const begin = attempt ? Date.parse(attempt.started_at) : null
  let cursor = begin
  const steps: StepView[] = names.map((name) => {
    const s = done.get(name)
    if (s) {
      const end = Date.parse(s.at)
      const startedAt = Number.isNaN(end) ? null : end - s.duration_ms
      if (!Number.isNaN(end)) cursor = end
      return {
        name,
        state: s.status,
        detail: s.detail,
        durationMs: s.duration_ms,
        startedAt,
      }
    }
    const running = attempt?.current_step === name
    return {
      name,
      state: running ? 'running' : 'pending',
      detail: '',
      durationMs: 0,
      startedAt: running ? cursor : null,
    }
  })

  const byPhase = new Map<PhaseId, StepView[]>()
  let prev: PhaseId | null = null
  for (const st of steps) {
    const id = phaseOf(st.name, prev)
    prev = id
    byPhase.set(id, [...(byPhase.get(id) ?? []), st])
  }
  const ids = [...PHASE_ORDER, 'other' as PhaseId].filter((id) =>
    byPhase.has(id),
  )
  const phases: PhaseView[] = ids.map((id) => {
    const list = byPhase.get(id) ?? []
    return {
      id,
      state: phaseState(list),
      steps: list,
      durationMs: list.reduce((n, s) => n + s.durationMs, 0),
    }
  })

  const counted = steps.filter((s) => s.name !== ROLLBACK)
  const settled = counted.filter(
    (s) => s.state === 'ok' || s.state === 'skipped',
  ).length
  const outcome = attempt?.outcome
  const percent =
    !attempt || counted.length === 0
      ? null
      : outcome === 'succeeded'
        ? 100
        : Math.min(99, Math.round((settled / counted.length) * 100))
  const current = steps.find(
    (s) => s.state === 'running' || s.state === 'failed',
  )
  const idx = current ? steps.indexOf(current) : settled
  return {
    phases,
    percent,
    stepIndex: Math.min(counted.length, idx + 1),
    total: counted.length,
    currentStep: current?.name ?? '',
    currentPhase: current ? phaseOf(current.name, null) : null,
    rolledBack: outcome === 'rolled_back',
  }
}
