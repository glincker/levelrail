export type ShellStatus =
  'healthy' | 'update' | 'degraded' | 'incident' | 'busy' | 'unknown'

export interface ShellStatusInput {
  level: 'ok' | 'warning' | 'critical' | null
  updateAvailable: boolean
  upgrading: boolean
  restarting: boolean
}

// Busy beats everything: during an upgrade the health signals are expected to flap.
export function deriveShellStatus(i: ShellStatusInput): ShellStatus {
  if (i.upgrading || i.restarting) return 'busy'
  if (i.level === 'critical') return 'incident'
  if (i.level === 'warning') return 'degraded'
  if (i.level === null) return 'unknown'
  return i.updateAvailable ? 'update' : 'healthy'
}
