import type { AdminState, LiveUpstream } from '../../queries/loadBalancerLive'
import { lbT } from '../../lib/lbText'

export interface AdminAction {
  state: AdminState
  label: string
  meaning: string
  disabled: boolean
}

export function unsupportedHint(): string {
  return lbT('admin.unsupported', 'Needs a newer control plane')
}

export function adminActions(
  u: LiveUpstream,
  supported: boolean,
): AdminAction[] {
  const current = u.admin_state ?? 'active'
  const all: Omit<AdminAction, 'disabled'>[] = [
    {
      state: 'active',
      label: lbT('admin.active.label', 'Enable'),
      meaning: lbT(
        'admin.active.meaning',
        'Back in rotation and receiving new requests.',
      ),
    },
    {
      state: 'draining',
      label: lbT('admin.draining.label', 'Drain'),
      meaning: lbT(
        'admin.draining.meaning',
        'Finish open requests, send no new ones.',
      ),
    },
    {
      state: 'disabled',
      label: lbT('admin.disabled.label', 'Disable'),
      meaning: lbT(
        'admin.disabled.meaning',
        'Out of rotation until you enable it again.',
      ),
    },
  ]
  return all
    .filter((a) => a.state !== current)
    .map((a) => ({ ...a, disabled: !supported }))
}
