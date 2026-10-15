import type { SelfUpgradeAttempt } from '../../queries/selfUpgrade'

export type RowState = 'pending' | 'running' | 'ok' | 'failed' | 'skipped'

export interface Row {
  name: string
  state: RowState
  detail: string
  durationMs: number
}

const GUARD = 'guard'
const ROLLBACK = 'rollback'

export function progressRows(
  planned: string[],
  attempt: SelfUpgradeAttempt | undefined,
): Row[] {
  const names = [GUARD, ...planned.filter((n) => n !== GUARD)]
  const done = new Map((attempt?.steps ?? []).map((s) => [s.name, s]))
  if (done.has(ROLLBACK)) names.push(ROLLBACK)
  return names.map((name) => {
    const s = done.get(name)
    if (s) {
      return {
        name,
        state: s.status,
        detail: s.detail,
        durationMs: s.duration_ms,
      }
    }
    const running = attempt?.current_step === name
    return {
      name,
      state: running ? 'running' : 'pending',
      detail: '',
      durationMs: 0,
    }
  })
}
