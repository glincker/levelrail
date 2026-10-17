// Wire types for a node's trust summary and security events
// (internal/api/node_trust.go).

export type NodeTrustState =
  | 'verified'
  | 'rejected'
  | 'revoked'
  | 'expired'
  | 'idle'
  | 'local'
  | 'no_activity'

export type CAPinState = 'pinned' | 'trusted_ca' | 'unpinned'

export interface NodeTrustResource {
  node_id: string
  is_local: boolean
  status: { state: NodeTrustState; reason?: string }
  ca?: {
    fingerprint?: string
    pin_state?: CAPinState
    pin_reported_at?: string
  }
  certificate?: {
    fingerprint?: string
    serial?: string
    issued_at?: string
    not_after?: string
    days_remaining?: number
    lifetime_used?: number
    key_origin: string
  }
  renewal?: {
    next_renew_at?: string
    next_renew_estimated?: boolean
    last_renewed_at?: string
    renewal_count: number
    last_error?: string
    last_failed_at?: string
    next_retry_at?: string
    consecutive_failures: number
  }
  validation?: {
    window_hours: number
    accepted: number
    rejected: number
    last_accepted_at?: string
    last_rejected_at?: string
  }
}

export type SecurityOutcome =
  'accepted' | 'rejected' | 'warning' | 'failed' | 'info'

export interface NodeSecurityEvent {
  id: number
  kind: string
  outcome: SecurityOutcome
  reason?: string
  source_ip?: string
  count: number
  first_at: string
  last_at: string
  token_id?: string
  subject?: string
  message?: string
  serial?: string
  fingerprint?: string
  not_before?: number
  not_after?: number
  actor?: string
}
