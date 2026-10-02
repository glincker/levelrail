// Wire types for the network share resource, matching
// internal/api/network_shares.go's networkShareResource /
// createNetworkShareRequest exactly. Password is write-only, present on
// the create/update request and nowhere else: never echoed back.

export type NetworkShareProtocol = 'nfs' | 'cifs'

export interface NetworkShare {
  id: string
  name: string
  protocol: NetworkShareProtocol
  host: string
  remote_path: string
  mount_options?: string
  username?: string
  created_at: string
}

export interface CreateNetworkShareRequest {
  name: string
  protocol: NetworkShareProtocol
  host: string
  remote_path: string
  mount_options?: string
  username?: string
  password?: string
}

export interface UpdateNetworkShareRequest {
  name: string
  protocol: NetworkShareProtocol
  host: string
  remote_path: string
  mount_options?: string
  username?: string
  password?: string
}
