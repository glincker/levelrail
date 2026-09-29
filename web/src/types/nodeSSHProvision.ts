// Wire types for SSH-based node provisioning, POST
// /api/v1/nodes/ssh-provision and GET /api/v1/ssh-node-provisions[/...]
// (internal/api/node_ssh_provision.go). Adopts a machine the operator
// already has instead of creating one at a cloud provider (see
// nodeProvision.ts for that flow); the two share no wire types since the
// SSH path has no provider/region/size and instead reports what it
// detected live over the SSH session.

// The six states store.SSHNodeProvision's own status column can hold.
// Unlike NodeProvisionStatus (recomputed live against a provider API),
// these are pushed by the provisioning goroutine itself as the SSH
// session progresses, so "installing" is actually emitted here (the
// cloud path's own status of the same name is defined but never used,
// see nodeProvision.ts's own doc comment on that).
export type SSHNodeProvisionStatus =
  'connecting' | 'detecting' | 'installing' | 'enrolling' | 'ready' | 'failed'

export interface SSHNodeProvisionResource {
  id: string
  name: string
  role: string
  status: SSHNodeProvisionStatus
  detected_os?: string
  detected_arch?: string
  node_id?: string
  failure_reason?: string
  log?: string
  created_at: string
  updated_at: string
}

export function isTerminalSSHProvisionStatus(
  status: SSHNodeProvisionStatus,
): boolean {
  return status === 'ready' || status === 'failed'
}
