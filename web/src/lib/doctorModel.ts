import type { DoctorCheck, DoctorStreamEvent } from '../queries/systemDoctor'
import type { Fix } from '../queries/fixit'

export const DOCTOR_GROUPS = [
  'runtime',
  'host',
  'network',
  'security',
  'backups',
  'nodes',
  'hardware',
] as const

export type DoctorGroupId = (typeof DOCTOR_GROUPS)[number] | 'other'

export interface DoctorState {
  /** Results keyed by the registered check that produced them. */
  byCheck: Record<string, DoctorCheck[]>
  total: number
  finished: number
  running: boolean
  error: string | null
}

export const initialDoctorState: DoctorState = {
  byCheck: {},
  total: 0,
  finished: 0,
  running: false,
  error: null,
}

export function startDoctorState(): DoctorState {
  return { ...initialDoctorState, running: true }
}

/** reduceDoctor folds one SSE event into the page state. */
export function reduceDoctor(
  state: DoctorState,
  ev: DoctorStreamEvent,
): DoctorState {
  switch (ev.type) {
    case 'start':
      return { ...state, total: ev.total ?? 0 }
    case 'check':
      return {
        ...state,
        byCheck: { ...state.byCheck, [ev.id ?? '']: ev.checks ?? [] },
        finished: state.finished + 1,
      }
    case 'fixes': {
      const fixed = new Map((ev.checks ?? []).map((c) => [c.code, c]))
      const byCheck: Record<string, DoctorCheck[]> = {}
      for (const [id, checks] of Object.entries(state.byCheck)) {
        byCheck[id] = checks.map((c) => {
          const f = fixed.get(c.code)
          return f ? { ...c, fix_id: f.fix_id, fix_kind: f.fix_kind } : c
        })
      }
      return { ...state, byCheck }
    }
    case 'done':
      return { ...state, running: false }
  }
}

export function allChecks(state: DoctorState): DoctorCheck[] {
  return Object.values(state.byCheck).flat()
}

const STATUS_RANK: Record<DoctorCheck['status'], number> = {
  fail: 0,
  warn: 1,
  unknown: 2,
  ok: 3,
}

export interface DoctorGroup {
  id: DoctorGroupId
  checks: DoctorCheck[]
}

/** groupChecks orders groups as the server lists them and puts problems first inside each. */
export function groupChecks(checks: DoctorCheck[]): DoctorGroup[] {
  const known = new Set<string>(DOCTOR_GROUPS)
  const buckets = new Map<DoctorGroupId, DoctorCheck[]>()
  for (const c of checks) {
    const id: DoctorGroupId =
      c.group && known.has(c.group) ? (c.group as DoctorGroupId) : 'other'
    buckets.set(id, [...(buckets.get(id) ?? []), c])
  }
  const order: DoctorGroupId[] = [...DOCTOR_GROUPS, 'other']
  return order
    .filter((id) => buckets.has(id))
    .map((id) => ({
      id,
      checks: [...(buckets.get(id) ?? [])].sort(
        (a, b) =>
          STATUS_RANK[a.status] - STATUS_RANK[b.status] ||
          a.name.localeCompare(b.name),
      ),
    }))
}

export interface SeverityCounts {
  fail: number
  warn: number
  unknown: number
  ok: number
}

export function countSeverity(checks: DoctorCheck[]): SeverityCounts {
  const out: SeverityCounts = { fail: 0, warn: 0, unknown: 0, ok: 0 }
  for (const c of checks) out[c.status] += 1
  return out
}

/** safeAutoFixes are the fixes "Fix all safe issues" may apply: one click, reversible or harmless, nothing to choose. */
export function safeAutoFixes(fixes: Fix[]): Fix[] {
  return fixes.filter(
    (f) =>
      f.failing &&
      f.kind === 'auto' &&
      f.risk === 'safe' &&
      f.state === 'offer' &&
      !f.param,
  )
}
