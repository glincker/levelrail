// Wire types for silences, maintenance windows and alert history,
// matching internal/api/alert_noise.go's resources field for field.

export type Severity = 'info' | 'warning' | 'critical'

export interface SilenceMatchers {
  rule_ids?: string[]
  kinds?: string[]
  apps?: string[]
  nodes?: string[]
  labels?: Record<string, string>
  severities?: Severity[]
}

export type SilenceStatus = 'pending' | 'active' | 'expired'

export interface Silence {
  id: string
  matchers: SilenceMatchers
  starts_at: string
  ends_at: string
  created_by: string
  reason?: string
  created_at: string
  expired_at?: string
  status: SilenceStatus
}

export interface CreateSilenceRequest {
  matchers: SilenceMatchers
  duration: string
  reason?: string
}

export type MaintenanceScope = 'all' | 'app' | 'node'

export interface MaintenanceWindow {
  id?: string
  name: string
  cron: string
  duration: string
  timezone?: string
  scope: MaintenanceScope
  targets?: string[]
  enabled: boolean
  created_by?: string
  active?: boolean
  active_until?: string
  next_start?: string
}

export type AlertOutcome =
  | 'sent'
  | 'silenced'
  | 'grouped'
  | 'inhibited'
  | 'failed'
  | 'ratelimited'
  | 'flapping'
  | 'skipped'

export type AlertEventName = 'fired' | 'resolved' | 'flapping' | 'flap_ended'

export interface AlertHistoryEntry {
  id: string
  at: string
  rule_id: string
  rule_name: string
  rule_kind: string
  resource_id?: string
  app?: string
  node?: string
  severity?: Severity
  event: AlertEventName
  outcome: AlertOutcome
  detail?: string
  silence_id?: string
  channel_id?: string
  error?: string
}

export interface AlertHistoryFilter {
  app?: string
  outcome?: AlertOutcome | ''
  event?: AlertEventName | ''
  limit?: number
}

// Quick silence durations offered from an alert row and the dashboard.
export const QUICK_SILENCE_DURATIONS = ['1h', '4h', '24h'] as const
