// Wire types for internal/api/audit_chain.go and audit_sinks.go. Sink
// credentials are write-only: only the signing secret of a new or rotated
// webhook sink is ever returned, once.

export type AuditSinkKind = 'syslog' | 'webhook' | 's3'

export interface AuditChainBreak {
  seq: number
  id?: string
  reason: string
}

export interface AuditChainReport {
  ok: boolean
  chained: number
  unchained: number
  head_seq: number
  head_hash: string
  first_broken?: AuditChainBreak
}

export interface AuditSinkConfig {
  address?: string
  tls?: boolean
  url?: string
  endpoint?: string
  region?: string
  bucket?: string
  prefix?: string
  path_style?: boolean
}

export interface AuditSink {
  id: string
  name: string
  kind: AuditSinkKind
  enabled: boolean
  config: AuditSinkConfig
  cursor_seq: number
  consecutive_failures: number
  next_attempt_at?: string
  last_error?: string
  last_delivered_at?: string
  created_at: string
  updated_at: string
  signing_secret?: string
}

export interface AuditSinkDelivery {
  id: string
  sink_id: string
  from_seq: number
  to_seq: number
  entries: number
  state: 'delivered' | 'failed'
  attempt: number
  last_error?: string
  created_at: string
}

export interface CreateAuditSinkRequest {
  name: string
  kind: AuditSinkKind
  enabled: boolean
  config: AuditSinkConfig
  access_key_id?: string
  secret_access_key?: string
  backfill?: boolean
}

export interface UpdateAuditSinkRequest {
  name?: string
  enabled?: boolean
  config?: AuditSinkConfig
  access_key_id?: string
  secret_access_key?: string
}

export interface AuditSinkTestResult {
  ok: boolean
  error?: string
}
