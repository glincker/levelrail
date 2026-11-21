import type { CloudInstance, CloudInstanceAction } from '../types/cloudAccounts'

const POWER_ACTIONS: readonly CloudInstanceAction[] = [
  'start',
  'stop',
  'soft_stop',
  'reboot',
  'soft_reboot',
]

const RUNNING = new Set(['running'])
const STOPPED = new Set(['stopped'])

// The matrix from the API is the source of truth; state only hides what cannot apply.
export function availableActions(
  inst: Pick<CloudInstance, 'state' | 'managed' | 'read_only'>,
  verbs: readonly string[],
  canOperate: boolean,
): CloudInstanceAction[] {
  if (!canOperate || !inst.managed || inst.read_only) return []
  return POWER_ACTIONS.filter((a) => {
    if (!verbs.includes(a)) return false
    if (a === 'start') return STOPPED.has(inst.state)
    return RUNNING.has(inst.state)
  })
}

export function needsConfirm(action: CloudInstanceAction): boolean {
  return action === 'stop' || action === 'soft_stop'
}

// Terminate is root only and needs the exact instance name (id when unnamed).
export function terminateName(inst: Pick<CloudInstance, 'id' | 'name'>) {
  return inst.name.trim() !== '' ? inst.name : inst.id
}

export function canTerminate(
  isRoot: boolean,
  inst: Pick<CloudInstance, 'managed' | 'read_only' | 'state'>,
  verbs: readonly string[],
): boolean {
  return (
    isRoot &&
    inst.managed &&
    !inst.read_only &&
    inst.state !== 'terminated' &&
    inst.state !== 'terminating' &&
    verbs.includes('terminate')
  )
}

export function instanceTone(
  state: string,
): 'success' | 'warning' | 'neutral' | 'danger' {
  if (state === 'running') return 'success'
  if (state === 'pending' || state === 'stopping' || state === 'terminating')
    return 'warning'
  if (state === 'unknown') return 'danger'
  return 'neutral'
}

export function instanceIps(inst: Pick<CloudInstance, 'public_ipv4'>): string {
  return inst.public_ipv4 ?? '-'
}

// Tags the user may edit as rows; empty keys are dropped, empty values remove the tag.
export function tagsToRows(tags: Record<string, string>): [string, string][] {
  return Object.entries(tags).sort(([a], [b]) => a.localeCompare(b))
}
