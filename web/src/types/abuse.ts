// Per-app ingress abuse defense (internal/api/app_abuse.go).

export type AbuseMode = 'off' | 'log_only' | 'enforce'

export type AbuseReason =
  'rate_limit' | 'deny_cidr' | 'deny_user_agent' | 'missing_user_agent'

export interface AbusePolicy {
  mode: AbuseMode
  rps?: number
  burst?: number
  deny_cidrs?: string[]
  deny_user_agents?: string[]
  require_user_agent?: boolean
}

export interface AbuseCondition {
  status: 'True' | 'False' | 'Unknown'
  reason: string
  message: string
  since?: string
}

export interface AbuseReasonCount {
  reason: AbuseReason
  would_block: number
  blocked: number
}

/** AbuseCounters is the defense's tally over a traffic window. */
export interface AbuseCounters {
  allowed: number
  would_block: number
  blocked: number
  fail_open: number
  by_reason: AbuseReasonCount[]
}

export interface AbuseSuggestion {
  peak_rps: number
  rps: number
  burst: number
  from_traffic: boolean
}

export interface AppAbuse {
  app: string
  configured: boolean
  policy: AbusePolicy
  updated_at?: string
  updated_by?: string
  condition: AbuseCondition
  suggestion: AbuseSuggestion
  counters: AbuseCounters
  counters_window_minutes: number
  limits: {
    max_rps: number
    max_burst: number
    max_deny_entries: number
    max_agent_entries: number
    dry_run_default_minutes: number
    dry_run_max_minutes: number
  }
}

export interface AbuseClientImpact {
  client: string
  requests: number
  would_block: number
  top_reason: AbuseReason
}

export interface AbuseDryRun {
  requests: number
  would_block: number
  percent: number
  by_reason: { reason: AbuseReason; count: number }[]
  top_clients: AbuseClientImpact[]
  window_minutes: number
  covered_minutes: number
  note: string
}
