import type { AdminState, LiveUpstream } from '../../queries/loadBalancerLive'

export interface AdminAction {
  state: AdminState
  label: string
  meaning: string
  disabled: boolean
}

export const UNSUPPORTED_HINT = 'Needs a newer control plane'

export function adminActions(
  u: LiveUpstream,
  supported: boolean,
): AdminAction[] {
  const current = u.admin_state ?? 'active'
  const all: Omit<AdminAction, 'disabled'>[] = [
    {
      state: 'active',
      label: 'Enable',
      meaning: 'Back in rotation and receiving new requests.',
    },
    {
      state: 'draining',
      label: 'Drain',
      meaning: 'Finish open requests, send no new ones.',
    },
    {
      state: 'disabled',
      label: 'Disable',
      meaning: 'Out of rotation until you enable it again.',
    },
  ]
  return all
    .filter((a) => a.state !== current)
    .map((a) => ({ ...a, disabled: !supported }))
}
