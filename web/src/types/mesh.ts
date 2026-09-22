// Wire types for GET /api/v1/mesh and POST /api/v1/nodes/{id}/mesh/rotate-key
// (internal/api/mesh.go's meshStatusResource/meshPeerResource/
// meshRotationResource/rotateKeyResponse). Same snake_case-matches-wire-
// shape convention nodeDetail.ts documents.

// One WireGuard peer as this control plane's own device sees it, joined
// with the node registry for a name and any store-recorded mesh address.
// `live` distinguishes a peer this node's device actually has a UAPI
// entry for (real handshake/transfer data) from one the store merely
// knows about but the mesh reconciler hasn't peered with yet: UI code
// must not render handshake/health fields for a non-live peer as if they
// were a stalled connection, they are simply not established yet.
export interface MeshPeerResource {
  node_id: string
  name?: string
  public_key: string
  mesh_address?: string
  endpoint?: string
  last_handshake_at?: string
  healthy: boolean
  transfer_rx_bytes: number
  transfer_tx_bytes: number
  live: boolean
}

// The local node's most recently tracked key rotation. Absent
// (MeshStatusResource.rotation is undefined) when no rotation has
// happened since the control plane process last started: rotation state
// lives only in memory server-side (internal/network's Coordinator.RotateKey
// doc comment), not a claim that no rotation has ever happened.
export interface MeshRotationResource {
  node_id: string
  old_public_key: string
  new_public_key: string
  started_at: string
  confirmed: boolean
  confirmed_at?: string
}

export interface MeshStatusResource {
  enabled: boolean
  backend?: string
  interface?: string
  local_node_id?: string
  public_key?: string
  mesh_address?: string
  listen_port?: number
  peers: MeshPeerResource[]
  rotation?: MeshRotationResource
}

export interface RotateKeyResponse {
  node_id: string
  old_public_key: string
  new_public_key: string
}
