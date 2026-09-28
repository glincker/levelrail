// Wire types for cloud node provisioning, GET/POST
// /api/v1/node-providers[/...] and /api/v1/nodes/provision,
// /api/v1/node-provisions[/...] (internal/api/node_provision.go).

export interface NodeProviderResource {
  provider: string
  has_token: boolean
}

export interface NodeProviderRegionResource {
  id: string
  name: string
}

export interface NodeProviderSizeResource {
  id: string
  name: string
  vcpus: number
  memory_mb: number
  disk_gb: number
  price_monthly?: string
  currency?: string
}

// The six states store.NodeProvision's own status column can hold.
// "installing" is defined server-side for a future agent-reported
// signal but never actually emitted today: this control plane can't yet
// tell "still running cloud-init" apart from "waiting to dial in", so a
// running server is always reported as "enrolling" instead. See
// docs/node-provisioning.md.
export type NodeProvisionStatus =
  'creating' | 'booting' | 'installing' | 'enrolling' | 'ready' | 'failed'

export interface NodeProvisionResource {
  id: string
  provider: string
  region: string
  size: string
  name: string
  role: string
  status: NodeProvisionStatus
  ip_address?: string
  node_id?: string
  failure_reason?: string
  created_at: string
  updated_at: string
}

export function isTerminalProvisionStatus(
  status: NodeProvisionStatus,
): boolean {
  return status === 'ready' || status === 'failed'
}
