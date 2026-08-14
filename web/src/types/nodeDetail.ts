// Wire type for a node, GET /api/v1/nodes and GET /api/v1/nodes/{id}
// (internal/api/nodes.go's nodeResource). Same snake_case-matches-wire-
// shape convention appDetail.ts and databaseDetail.ts document: an
// internal admin dashboard talking to one frozen backend contract, not
// a public SDK.

export type NodeStatus = 'pending' | 'online' | 'offline' | 'cordoned'

export interface NodeResource {
  id: string
  name: string
  address?: string
  status: NodeStatus
  cert_fingerprint?: string
  joined_at?: string
  last_seen_at?: string
  created_at: string
}

// Response body for POST /api/v1/nodes/join-tokens
// (internal/api/nodes.go's createNodeJoinTokenResponse): the token's one
// and only appearance in plaintext, the same "shown once" shape a
// created API token already has (see queries/auth.ts's own token
// handling).
export interface NodeJoinTokenResponse {
  token: string
  expires_at: string
}
