// Wire types for a node's agent certificate lifecycle and agent build
// (internal/api/node_cert.go, ADR 021).

export type NodeCertState =
  'ok' | 'expiring' | 'critical' | 'expired' | 'revoked' | 'unknown'

export interface NodeCertResource {
  state: NodeCertState
  not_after?: string
  days_remaining?: number
  renewed_at?: string
  generation: number
  // 'agent' when the node generated its own key, 'reenroll_required' when
  // an early beta had the control plane make it.
  key_origin: string
  previous_valid_until?: string
  revoked_at?: string
  warning_days: number
  critical_days: number
  renewal?: NodeCertRenewal
}

// The agent's last reported renewal failure, already bounded by the server.
export interface NodeCertRenewal {
  last_error: string
  failed_at?: string
  next_retry_at?: string
  consecutive_failures: number
}

export interface NodeAgentResource {
  version?: string
  commit?: string
  os?: string
  arch?: string
  reported_at?: string
  outdated: boolean
  min_version?: string
  control_plane_version: string
}

export interface NodeReenrollTokenResponse {
  token: string
  node_id: string
  expires_at: string
  ca_fingerprint?: string
  agent_binary: string
  control_plane_address?: string
}
