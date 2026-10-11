import type { MeshSettings } from '../queries/meshSettings'

export type MeshCardState = 'off' | 'on' | 'enabling' | 'disabling' | 'error'

// One state for the badge, derived from what is configured versus what runs.
export function meshCardState(s: MeshSettings): MeshCardState {
  if (s.start_error && s.configured) return 'error'
  if (s.configured && !s.effective) return 'enabling'
  if (!s.configured && s.effective) return 'disabling'
  return s.effective ? 'on' : 'off'
}
